// Package extractor uses mauidude/go-readability and local rules to get articles
package extractor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
	"github.com/mauidude/go-readability"
	"github.com/sashabaranov/go-openai"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ukeeper/ukeeper-readability/datastore"
)

const (
	defaultMaxGPTIter = 3
	aiEvaluatorUser   = "ai-evaluator"
)

//go:generate moq -out mocks/rules.go -pkg mocks -skip-ensure -fmt goimports . Rules
//go:generate moq -out mocks/openai_client.go -pkg mocks -skip-ensure -fmt goimports . OpenAIClient
//go:generate moq -out mocks/summaries.go -pkg mocks -skip-ensure -fmt goimports . Summaries

// OpenAIClient defines interface for OpenAI API client
type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// Rules interface with all methods to access datastore
type Rules interface {
	Get(ctx context.Context, rURL string) (datastore.Rule, bool)
	GetByID(ctx context.Context, id bson.ObjectID) (datastore.Rule, bool)
	Save(ctx context.Context, rule datastore.Rule) (datastore.Rule, error)
	Disable(ctx context.Context, id bson.ObjectID) error
	All(ctx context.Context) []datastore.Rule
}

// Summaries interface with all methods to access summary cache
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
	Rules            Rules
	Retriever        Retriever
	AIEvaluator      AIEvaluator
	MaxGPTIter       int
	Summaries        Summaries
	OpenAIKey        string
	ModelType        string
	OpenAIEnabled    bool
	SummaryPrompt    string
	MaxContentLength int
	RequestsPerMin   int

	defaultRetrieverOnce sync.Once
	defaultRetriever     Retriever
	apiClient            OpenAIClient
	rateLimiter          *time.Ticker
	requestsMutex        sync.Mutex
	metrics              SummaryMetrics
	metricsMutex         sync.RWMutex
}

// SetAPIClient sets the API client for testing purposes
func (f *UReadability) SetAPIClient(client OpenAIClient) {
	f.apiClient = client
}

// StartCleanupTask starts a background task to periodically clean up expired summaries
func (f *UReadability) StartCleanupTask(ctx context.Context, interval time.Duration) {
	if f.Summaries == nil {
		log.Print("[WARN] summaries store is not configured, cleanup task not started")
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
				log.Print("[INFO] running expired summaries cleanup task")
				count, err := f.Summaries.CleanupExpired(ctx)
				if err != nil {
					log.Printf("[ERROR] failed to clean up expired summaries: %v", err)
				} else {
					log.Printf("[INFO] cleaned up %d expired summaries", count)
				}
			case <-ctx.Done():
				log.Print("[INFO] stopping summaries cleanup task")
				return
			}
		}
	}()
	log.Printf("[INFO] started summaries cleanup task with interval %v", interval)
}

// retriever returns the configured Retriever, defaulting to a cached HTTPRetriever if nil
func (f *UReadability) retriever() Retriever {
	if f.Retriever != nil {
		return f.Retriever
	}
	f.defaultRetrieverOnce.Do(func() {
		f.defaultRetriever = &HTTPRetriever{Timeout: f.TimeOut}
	})
	return f.defaultRetriever
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
	DefaultSummaryPrompt = "You are a helpful assistant that summarizes articles. " +
		"Please summarize the main points in a few sentences as TLDR style (don't add a TLDR label). " +
		"Then, list up to five detailed bullet points. Provide the response in plain text. " +
		"Do not add any additional information. Do not add a Summary at the beginning of the response. " +
		"If detailed bullet points are too similar to the summary, don't include them at all:"
)

// Extract fetches page and retrieves article
func (f *UReadability) Extract(ctx context.Context, reqURL string) (*Response, error) {
	return f.extractWithRules(ctx, reqURL, nil, false)
}

// ExtractByRule fetches page and retrieves article using a specific rule
func (f *UReadability) ExtractByRule(ctx context.Context, reqURL string, rule *datastore.Rule) (*Response, error) {
	return f.extractWithRules(ctx, reqURL, rule, false)
}

// ExtractAndImprove fetches page and re-extracts using general parser, then evaluates with AI.
// Used when a user reports that extraction for a URL is bad — ignores existing rules.
func (f *UReadability) ExtractAndImprove(ctx context.Context, reqURL string) (*Response, error) {
	return f.extractWithRules(ctx, reqURL, nil, true)
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
	// check if openai summarisation is enabled
	if !f.OpenAIEnabled {
		return "", errors.New("summary generation is disabled")
	}

	// check if api key is available
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
				log.Print("[DEBUG] using cached summary for content")

				// track cache hit
				f.metricsMutex.Lock()
				f.metrics.CacheHits++
				f.metricsMutex.Unlock()

				return cachedSummary.Summary, nil
			}

			log.Print("[DEBUG] cached summary has expired, regenerating")
		}
	}

	// track cache miss
	f.metricsMutex.Lock()
	f.metrics.CacheMisses++
	f.metrics.TotalRequests++
	f.metricsMutex.Unlock()

	// apply content length limit if configured
	if f.MaxContentLength > 0 && len(content) > f.MaxContentLength {
		log.Printf("[DEBUG] content length (%d) exceeds maximum allowed (%d), truncating",
			len(content), f.MaxContentLength)
		content = content[:f.MaxContentLength] + "..."
	}

	// initialize api client if not already set
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
			log.Print("[DEBUG] rate limiter allowed request")
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

	// make the api request
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
		log.Printf("[WARN] AI summarisation failed: %v", err)

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
			log.Print("[DEBUG] summary cached successfully")
		}
	}

	return summary, nil
}

// extractWithRules is the core function that handles extraction with or without a specific rule.
// when force=true, the initial extraction uses the general parser (ignores stored rules),
// and evaluation is always triggered regardless of existing rules.
func (f *UReadability) extractWithRules(ctx context.Context, reqURL string, rule *datastore.Rule, force bool) (*Response, error) { //nolint:revive // force flag is intentional, controls extraction mode
	log.Printf("[INFO] extract %s (force=%v)", reqURL, force)
	rb := &Response{}

	result, err := f.retriever().Retrieve(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	rb.URL = result.URL

	var body string
	rb.ContentType, rb.Charset, body = f.toUtf8(result.Body, result.Header)

	if force {
		// force mode: use general parser, skip stored rules entirely
		rb.Content, rb.Rich, err = f.getContentGeneral(body)
	} else {
		rb.Content, rb.Rich, err = f.getContent(ctx, body, reqURL, rule)
	}
	if err != nil {
		log.Printf("[WARN] failed to parse %s, error=%v", reqURL, err)
		return nil, err
	}

	dbody, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	rb.Title = dbody.Find("title").First().Text()

	finalURL, err := url.Parse(rb.URL)
	if err != nil {
		return nil, fmt.Errorf("parse final URL %q: %w", rb.URL, err)
	}
	rb.Domain = finalURL.Host

	rb.Content = f.getText(rb.Content, rb.Title)
	rb.Rich, rb.AllLinks = f.normalizeLinks(rb.Rich, finalURL)
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

	// run AI evaluation if configured
	if f.AIEvaluator != nil {
		shouldEvaluate := force // always evaluate in force mode
		if !force {
			// in normal mode, only evaluate when domain has no existing rule;
			// use final URL (after redirects) so the check matches the domain where rules are saved
			if f.Rules != nil {
				if _, found := f.Rules.Get(ctx, rb.URL); !found {
					shouldEvaluate = true
				}
			} else {
				shouldEvaluate = true
			}
		}
		if shouldEvaluate {
			rb = f.evaluateAndImprove(ctx, reqURL, body, rb)
		}
	}

	log.Printf("[INFO] completed for %s, url=%s", rb.Title, rb.URL)
	return rb, nil
}

func (f *UReadability) maxGPTIter() int {
	if f.MaxGPTIter > 0 {
		return f.MaxGPTIter
	}
	return defaultMaxGPTIter
}

// evaluateAndImprove runs the AI evaluation loop. It sends the current extraction to the evaluator,
// and if the evaluator suggests a CSS selector, tries it on the HTML body. Iterates up to MaxGPTIter times.
// If a better selector is found, saves it as a rule. All errors are logged and swallowed — the original
// result is returned unchanged on any failure.
func (f *UReadability) evaluateAndImprove(ctx context.Context, reqURL, htmlBody string, result *Response) *Response {
	best := result
	var bestSelector string

	maxIter := f.maxGPTIter()
	log.Printf("[INFO] starting AI evaluation for %s, max iterations=%d", reqURL, maxIter)

	var lastTriedSelector string // tracks last attempted selector (including failed ones)
	for i := range maxIter {
		eval, err := f.AIEvaluator.Evaluate(ctx, reqURL, best.Content, htmlBody, lastTriedSelector)
		if err != nil {
			log.Printf("[WARN] AI evaluation error for %s on iteration %d: %v", reqURL, i, err)
			return best
		}

		if eval.Good {
			log.Printf("[INFO] AI evaluation: extraction is good for %s on iteration %d", reqURL, i)
			break
		}

		if eval.Selector == "" {
			log.Printf("[WARN] AI evaluation: bad extraction but no selector suggested for %s", reqURL)
			continue
		}

		log.Printf("[INFO] AI evaluation: trying selector %q for %s (iteration %d)", eval.Selector, reqURL, i)
		lastTriedSelector = eval.Selector

		// try the suggested selector on the HTML body
		rawHTML, err := f.extractWithSelector(htmlBody, eval.Selector)
		if err != nil || rawHTML == "" {
			log.Printf("[WARN] AI selector %q produced no content for %s: %v", eval.Selector, reqURL, err)
			continue
		}

		// rebuild the response with new content (defer link normalisation and image extraction to after the loop)
		improved := *best
		improved.Content = f.getText(rawHTML, best.Title)
		improved.Rich = rawHTML
		improved.Excerpt = f.getSnippet(improved.Content)

		best = &improved
		bestSelector = eval.Selector
	}

	// post-process the final result: normalise links and extract images once
	if bestSelector != "" {
		finalURL, err := url.Parse(best.URL)
		if err != nil {
			log.Printf("[WARN] failed to parse URL %q in evaluateAndImprove: %v", best.URL, err)
			return best
		}
		best.Rich, best.AllLinks = f.normalizeLinks(best.Rich, finalURL)
		darticle, err := goquery.NewDocumentFromReader(strings.NewReader(best.Rich))
		if err == nil {
			if im, allImages, ok := f.extractPics(darticle.Find("img"), reqURL); ok {
				best.Image = im
				best.AllImages = allImages
			}
		}
	}

	// save rule if we found a better selector
	if bestSelector != "" && f.Rules != nil {
		// merge with existing rule to preserve fields like TestURLs, MatchURLs, etc.
		rule, found := f.Rules.Get(ctx, best.URL)
		if !found {
			rule = datastore.Rule{Domain: best.Domain}
		}
		rule.Content = bestSelector
		rule.Enabled = true
		rule.User = aiEvaluatorUser
		if _, err := f.Rules.Save(ctx, rule); err != nil {
			log.Printf("[WARN] failed to save AI-suggested rule for %s: %v", best.Domain, err)
		} else {
			log.Printf("[INFO] saved AI-suggested rule for %s: %q", best.Domain, bestSelector)
		}
	}

	return best
}

// extractWithSelector applies a CSS selector to the HTML body and returns the raw extracted HTML
func (f *UReadability) extractWithSelector(htmlBody, selector string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return "", err
	}
	var res string
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		if html, err := s.Html(); err == nil {
			res += html
		}
	})
	return res, nil
}

// getContentGeneral extracts content using the general readability parser only,
// bypassing any stored rules. Used in force mode.
func (f *UReadability) getContentGeneral(body string) (content, rich string, err error) {
	doc, err := readability.NewDocument(body)
	if err != nil {
		return "", "", err
	}
	content, rich = doc.ContentWithHTML()
	return content, rich, nil
}

// getContent retrieves content from raw body string, both content (text only) and rich (with html tags)
// if rule is provided, it uses custom rule, otherwise tries to retrieve one from the storage,
// and at last tries to use general readability parser
func (f *UReadability) getContent(ctx context.Context, body, reqURL string, rule *datastore.Rule) (content, rich string, err error) {
	// custom rules parser
	customParser := func(body, reqURL string, rule datastore.Rule) (content, rich string, err error) {
		log.Printf("[DEBUG] custom extractor for %s", reqURL)
		res, err := f.extractWithSelector(body, rule.Content)
		if err != nil {
			return "", "", err
		}
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
		log.Print("[DEBUG] no rules defined!")
	}

	return f.getContentGeneral(body)
}

// makes all links absolute and returns all found links
func (f *UReadability) normalizeLinks(data string, baseURL *url.URL) (result string, links []string) {
	absoluteLink := func(link string) (absLink string, changed bool) {
		if r, err := baseURL.Parse(link); err == nil {
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
