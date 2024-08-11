// Package extractor uses mauidude/go-readability and local rules to get articles
package extractor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
	"github.com/mauidude/go-readability"
	"github.com/sashabaranov/go-openai"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/ukeeper/ukeeper-readability/backend/datastore"
)

//go:generate moq -out openai_mock.go . OpenAIClient
type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// Rules interface with all methods to access datastore
type Rules interface {
	Get(ctx context.Context, rURL string) (datastore.Rule, bool)
	GetByID(ctx context.Context, id primitive.ObjectID) (datastore.Rule, bool)
	Save(ctx context.Context, rule datastore.Rule) (datastore.Rule, error)
	Disable(ctx context.Context, id primitive.ObjectID) error
	All(ctx context.Context) []datastore.Rule
}

// UReadability implements fetcher & extractor for local readability-like functionality
type UReadability struct {
	TimeOut     time.Duration
	SnippetSize int
	Rules       Rules
	OpenAIKey   string

	openAIClient OpenAIClient
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
)

// Extract fetches page and retrieves article
func (f *UReadability) Extract(ctx context.Context, reqURL string) (*Response, error) {
	return f.extractWithRules(ctx, reqURL, nil)
}

// ExtractByRule fetches page and retrieves article using a specific rule
func (f *UReadability) ExtractByRule(ctx context.Context, reqURL string, rule *datastore.Rule) (*Response, error) {
	return f.extractWithRules(ctx, reqURL, rule)
}

func (f *UReadability) GenerateSummary(ctx context.Context, content string) (string, error) {
	if f.OpenAIKey == "" {
		return "", fmt.Errorf("OpenAI key is not set")
	}
	if f.openAIClient == nil {
		f.openAIClient = openai.NewClient(f.OpenAIKey)
	}
	resp, err := f.openAIClient.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT4o,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a helpful assistant that summarizes articles. Please summarize the main points in a few sentences as TLDR style (don't add a TLDR label). Then, list up to five detailed bullet points. Provide the response in plain text. Do not add any additional information. Do not add a Summary at the beginning of the response. If detailed bullet points are too similar to the summary, don't include them at all:",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: content,
				},
			},
		},
	)

	if err != nil {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

// ExtractWithRules is the core function that handles extraction with or without a specific rule
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
