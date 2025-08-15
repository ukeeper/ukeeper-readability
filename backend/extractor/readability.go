// Package extractor uses mauidude/go-readability and local rules to get articles
package extractor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
	"github.com/mauidude/go-readability"
	"github.com/sashabaranov/go-openai"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/ukeeper/ukeeper-readability/backend/datastore"
)

//go:generate moq -out openai_mock.go . OpenAIClient

// OpenAIClient defines interface for OpenAI API client
type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// rulesProvider interface with all methods to access datastore
type rulesProvider interface {
	Get(ctx context.Context, rURL string) (datastore.Rule, bool)
	GetByID(ctx context.Context, id primitive.ObjectID) (datastore.Rule, bool)
	Save(ctx context.Context, rule datastore.Rule) (datastore.Rule, error)
	Disable(ctx context.Context, id primitive.ObjectID) error
	All(ctx context.Context) []datastore.Rule
}

// Summaries interface with all methods to access summary cache
//
//go:generate moq -out summaries_mock.go . Summaries
type Summaries interface {
	Get(ctx context.Context, content string) (datastore.Summary, bool)
	Save(ctx context.Context, summary datastore.Summary) error
	Delete(ctx context.Context, contentHash string) error
	CleanupExpired(ctx context.Context) (int64, error)
}

// SummaryMetrics contains metrics related to summary generation
type SummaryMetrics struct {
	CacheHits          int64         `json:"cache_hits"`
	CacheMisses        int64         `json:"cache_misses"`
	TotalRequests      int64         `json:"total_requests"`
	FailedRequests     int64         `json:"failed_requests"`
	AverageResponseMs  int64         `json:"average_response_ms"`
	TotalResponseTimes time.Duration `json:"-"` // used to calculate average, not exported
}

// UReadability implements fetcher & extractor for local readability-like functionality
type UReadability struct {
	TimeOut          time.Duration
	SnippetSize      int
	Rules            rulesProvider
	Summaries        Summaries
	OpenAIKey        string
	ModelType        string
	OpenAIEnabled    bool
	SummaryPrompt    string
	MaxContentLength int
	RequestsPerMin   int

	apiClient     OpenAIClient
	rateLimiter   *time.Ticker
	requestsMutex sync.Mutex
	metrics       SummaryMetrics
	metricsMutex  sync.RWMutex
}

// SetAPIClient sets the API client for testing purposes
func (f *UReadability) SetAPIClient(client OpenAIClient) {
	f.apiClient = client
}

// StartCleanupTask starts a background task to periodically clean up expired summaries
func (f *UReadability) StartCleanupTask(ctx context.Context, interval time.Duration) {
	if f.Summaries == nil {
		log.Printf("[WARN] summaries store is not configured, cleanup task not started")
		return
	}

	if interval <= 0 {
		interval = 24 * time.Hour // default to daily cleanup
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Printf("[INFO] running expired summaries cleanup task")
				count, err := f.Summaries.CleanupExpired(ctx)
				if err != nil {
					log.Printf("[ERROR] failed to clean up expired summaries: %v", err)
				} else {
					log.Printf("[INFO] cleaned up %d expired summaries", count)
				}
			case <-ctx.Done():
				log.Printf("[INFO] stopping summaries cleanup task")
				return
			}
		}
	}()
	log.Printf("[INFO] started summaries cleanup task with interval %v", interval)
}

// Response from api calls
type Response struct {
	Summary     string   `json:"summary,omitempty"`
	Content     string   `json:"content"`
	Rich        string   `json:"rich_content"`
	Domain      string   `json:"domain"`
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Excerpt     string   `json:"excerpt"`
	Image       string   `json:"lead_image_url"`
	AllImages   []string `json:"images"`
	AllLinks    []string `json:"links"`
	ContentType string   `json:"type"`
	Charset     string   `json:"charset"`
}

var (
	reLinks  = regexp.MustCompile(`(href|src|action|background)="([^"]*)"`)
	reSpaces = regexp.MustCompile(`\s+`)
	reDot    = regexp.MustCompile(`\D(\.)\S`)
)

const (
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.4 Safari/605.1.15"

	// DefaultSummaryPrompt is the default prompt for generating article summaries
	DefaultSummaryPrompt = "You are a helpful assistant that summarizes articles. Please summarize the main points in a few sentences as TLDR style (don't add a TLDR label). Then, list up to five detailed bullet points. Provide the response in plain text. Do not add any additional information. Do not add a Summary at the beginning of the response. If detailed bullet points are too similar to the summary, don't include them at all:"
)

// Extract fetches page and retrieves article
func (f *UReadability) Extract(ctx context.Context, reqURL string) (*Response, error) {
	return f.extractWithRules(ctx, reqURL, nil)
}

// ExtractByRule fetches page and retrieves article using a specific rule
func (f *UReadability) ExtractByRule(ctx context.Context, reqURL string, rule *datastore.Rule) (*Response, error) {
	return f.extractWithRules(ctx, reqURL, rule)
}

// GetMetrics returns the current summary metrics
func (f *UReadability) GetMetrics() SummaryMetrics {
	f.metricsMutex.RLock()
	defer f.metricsMutex.RUnlock()

	// make a copy to ensure thread safety
	metrics := f.metrics

	// calculate average response time if we have any requests
	if metrics.TotalRequests > 0 {
		metrics.AverageResponseMs = int64(metrics.TotalResponseTimes/time.Millisecond) / metrics.TotalRequests
	}

	return metrics
}

// GenerateSummary creates a summary of the content using OpenAI
func (f *UReadability) GenerateSummary(ctx context.Context, content string) (string, error) {
	// check if OpenAI summarization is enabled
	if !f.OpenAIEnabled {
		return "", errors.New("summary generation is disabled")
	}

	// check if API key is available
	if f.OpenAIKey == "" {
		return "", errors.New("API key for summarization is not set")
	}

	// hash content for caching and detecting changes
	contentHash := datastore.GenerateContentHash(content)

	// check cache for existing summary
	if f.Summaries != nil {
		if cachedSummary, found := f.Summaries.Get(ctx, content); found {
			// check if summary is valid and not expired
			if cachedSummary.ExpiresAt.IsZero() || !time.Now().After(cachedSummary.ExpiresAt) {
				log.Printf("[DEBUG] using cached summary for content")

				// track cache hit
				f.metricsMutex.Lock()
				f.metrics.CacheHits++
				f.metricsMutex.Unlock()

				return cachedSummary.Summary, nil
			}

			log.Printf("[DEBUG] cached summary has expired, regenerating")
		}
	}

	// track cache miss
	f.metricsMutex.Lock()
	f.metrics.CacheMisses++
	f.metrics.TotalRequests++
	f.metricsMutex.Unlock()

	// apply content length limit if configured
	if f.MaxContentLength > 0 && len(content) > f.MaxContentLength {
		log.Printf("[DEBUG] content length (%d) exceeds maximum allowed (%d), truncating", len(content), f.MaxContentLength)
		content = content[:f.MaxContentLength] + "..."
	}

	// initialize API client if not already set
	if f.apiClient == nil {
		f.apiClient = openai.NewClient(f.OpenAIKey)
	}

	// initialize rate limiter if needed and configured
	f.requestsMutex.Lock()
	shouldThrottle := f.RequestsPerMin > 0 && f.OpenAIKey != ""
	if shouldThrottle && f.rateLimiter == nil {
		interval := time.Minute / time.Duration(f.RequestsPerMin)
		f.rateLimiter = time.NewTicker(interval)
	}
	f.requestsMutex.Unlock()

	// apply rate limiting if enabled
	if shouldThrottle {
		select {
		case <-f.rateLimiter.C:
			// continue with the request
			log.Printf("[DEBUG] rate limiter allowed request")
		case <-ctx.Done():
			// track failed request due to context cancellation
			f.metricsMutex.Lock()
			f.metrics.FailedRequests++
			f.metricsMutex.Unlock()
			return "", ctx.Err()
		}
	}

	// set the model to use
	model := openai.GPT4oMini
	if f.ModelType != "" {
		model = f.ModelType
	}

	// use custom prompt if provided, otherwise use default
	prompt := DefaultSummaryPrompt
	if f.SummaryPrompt != "" {
		prompt = f.SummaryPrompt
	}

	// track response time
	startTime := time.Now()

	// make the API request
	resp, err := f.apiClient.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: prompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: content,
				},
			},
		},
	)

	// calculate response time
	responseTime := time.Since(startTime)

	if err != nil {
		log.Printf("[WARN] AI summarization failed: %v", err)

		// track failed request
		f.metricsMutex.Lock()
		f.metrics.FailedRequests++
		f.metricsMutex.Unlock()

		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	// update metrics with response time
	f.metricsMutex.Lock()
	f.metrics.TotalResponseTimes += responseTime
	f.metricsMutex.Unlock()

	summary := resp.Choices[0].Message.Content

	// cache the summary if storage is available
	if f.Summaries != nil {
		// set expiration time to 1 month from now
		expiresAt := time.Now().AddDate(0, 1, 0)

		err = f.Summaries.Save(ctx, datastore.Summary{
			ID:        contentHash,
			Content:   content,
			Summary:   summary,
			Model:     model,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			ExpiresAt: expiresAt,
		})

		if err != nil {
			log.Printf("[WARN] failed to cache summary: %v", err)
		} else {
			log.Printf("[DEBUG] summary cached successfully")
		}
	}

	return summary, nil
}

// extractWithRules is the core function that handles extraction with or without a specific rule
func (f *UReadability) extractWithRules(ctx context.Context, reqURL string, rule *datastore.Rule) (*Response, error) {
	log.Printf("[INFO] extract %s", reqURL)
	rb := &Response{}

	httpClient := &http.Client{Timeout: f.TimeOut}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, http.NoBody)
	if err != nil {
		log.Printf("[WARN] failed to create request for %s, error=%v", reqURL, err)
		return nil, err
	}
	req.Close = true
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)

	if err != nil {
		log.Printf("[WARN] failed to get anything from %s, error=%v", reqURL, err)
		return nil, err
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			log.Printf("[WARN] failed to close response body, error=%v", err)
		}
	}()

	rb.URL = resp.Request.URL.String()
	dataBytes, e := io.ReadAll(resp.Body)

	if e != nil {
		log.Printf("[WARN] failed to read data from %s, error=%v", reqURL, e)
		return nil, e
	}

	var body string
	rb.ContentType, rb.Charset, body = f.toUtf8(dataBytes, resp.Header)
	rb.Content, rb.Rich, err = f.getContent(ctx, body, reqURL, rule)
	if err != nil {
		log.Printf("[WARN] failed to parse %s, error=%v", reqURL, err)
		return nil, err
	}

	dbody, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	rb.Title = dbody.Find("title").First().Text()

	if r, e := url.Parse(rb.URL); e == nil {
		rb.Domain = r.Host
	}

	rb.Content = f.getText(rb.Content, rb.Title)
	rb.Rich, rb.AllLinks = f.normalizeLinks(rb.Rich, resp.Request)
	rb.Excerpt = f.getSnippet(rb.Content)
	darticle, err := goquery.NewDocumentFromReader(strings.NewReader(rb.Rich))
	if err != nil {
		log.Printf("[WARN] failed to create document from reader, error=%v", err)
		return nil, err
	}
	if im, allImages, ok := f.extractPics(darticle.Find("img"), reqURL); ok {
		rb.Image = im
		rb.AllImages = allImages
	}

	log.Printf("[INFO] completed for %s, url=%s", rb.Title, rb.URL)
	return rb, nil
}

// getContent retrieves content from raw body string, both content (text only) and rich (with html tags)
// if rule is provided, it uses custom rule, otherwise tries to retrieve one from the storage,
// and at last tries to use general readability parser
func (f *UReadability) getContent(ctx context.Context, body, reqURL string, rule *datastore.Rule) (content, rich string, err error) {
	// general parser
	genParser := func(body, _ string) (content, rich string, err error) {
		doc, err := readability.NewDocument(body)
		if err != nil {
			return "", "", err
		}
		content, rich = doc.ContentWithHTML()
		return content, rich, nil
	}

	// custom rules parser
	customParser := func(body, reqURL string, rule datastore.Rule) (content, rich string, err error) {
		log.Printf("[DEBUG] custom extractor for %s", reqURL)
		dbody, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		if err != nil {
			return "", "", err
		}
		var res string
		dbody.Find(rule.Content).Each(func(_ int, s *goquery.Selection) {
			if html, err := s.Html(); err == nil {
				res += html
			}
		})
		if res == "" {
			return "", "", fmt.Errorf("nothing extracted from %s, rule=%v", reqURL, rule)
		}
		log.Printf("[INFO] custom rule processed for %s", reqURL)
		return f.getText(res, ""), res, nil
	}

	if rule != nil {
		log.Printf("[DEBUG] custom rule provided for %s: %v", reqURL, rule)
		return customParser(body, reqURL, *rule)
	}

	if f.Rules != nil {
		r := f.Rules
		if rule, found := r.Get(ctx, reqURL); found {
			if content, rich, err = customParser(body, reqURL, rule); err == nil {
				return content, rich, nil
			}
			log.Printf("[WARN] custom extractor failed for %s, error=%v", reqURL, err) // back to general parser
		}
	} else {
		log.Printf("[DEBUG] no rules defined!")
	}

	return genParser(body, reqURL)
}

// makes all links absolute and returns all found links
func (f *UReadability) normalizeLinks(data string, reqContext *http.Request) (result string, links []string) {
	absoluteLink := func(link string) (absLink string, changed bool) {
		if r, err := reqContext.URL.Parse(link); err == nil {
			return r.String(), r.String() != link
		}
		return "", false
	}

	result = data
	matches := reLinks.FindAllStringSubmatch(data, -1)
	normalizedCount := 0
	for _, m := range matches {
		srcLink := m[len(m)-1] // link in last element of the group
		dstLink := srcLink
		if absLink, changed := absoluteLink(srcLink); changed {
			dstLink = absLink
			srcLink = fmt.Sprintf("%q", srcLink)
			absLink = fmt.Sprintf("%q", absLink)
			result = strings.ReplaceAll(result, srcLink, absLink)
			log.Printf("[DEBUG] normalized %s -> %s", srcLink, dstLink)
			normalizedCount++
		}
		links = append(links, dstLink)
	}
	log.Printf("[DEBUG] normalized %d links", normalizedCount)
	return result, links
}
