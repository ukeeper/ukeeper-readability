// Package extractor uses mauidude/go-readability and local rules to get articles
package extractor

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
	"github.com/mauidude/go-readability"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ukeeper/ukeeper-readability/datastore"
)

//go:generate moq -out mocks/rules.go -pkg mocks -skip-ensure -fmt goimports . Rules

// Rules interface with all methods to access datastore
type Rules interface {
	Get(ctx context.Context, rURL string) (datastore.Rule, bool)
	GetByID(ctx context.Context, id bson.ObjectID) (datastore.Rule, bool)
	Save(ctx context.Context, rule datastore.Rule) (datastore.Rule, error)
	Disable(ctx context.Context, id bson.ObjectID) error
	All(ctx context.Context) []datastore.Rule
}

// UReadability implements fetcher & extractor for local readability-like functionality
type UReadability struct {
	TimeOut     time.Duration
	SnippetSize int
	Rules       Rules
	Retriever   Retriever // default retriever; when nil a cached HTTPRetriever is used
	CFRetriever Retriever // optional Cloudflare Browser Rendering retriever; when set, enables routing
	CFRouteAll  bool      // route every request through CFRetriever (requires CFRetriever != nil)

	defaultRetrieverOnce sync.Once
	defaultRetriever     Retriever
}

// retriever returns the configured default Retriever, creating a cached HTTPRetriever if nil
func (f *UReadability) retriever() Retriever {
	if f.Retriever != nil {
		return f.Retriever
	}
	f.defaultRetrieverOnce.Do(func() {
		f.defaultRetriever = &HTTPRetriever{Timeout: f.TimeOut}
	})
	return f.defaultRetriever
}

// pickRetriever decides which retriever should fetch the given URL based on routing config and an
// optional pre-resolved rule. Falls back to the default retriever unless CFRetriever is set AND
// either CFRouteAll is true or the rule explicitly asks for Cloudflare.
func (f *UReadability) pickRetriever(rule *datastore.Rule) Retriever {
	if f.CFRetriever == nil {
		return f.retriever()
	}
	if f.CFRouteAll {
		return f.CFRetriever
	}
	if rule != nil && rule.UseCloudflare {
		return f.CFRetriever
	}
	return f.retriever()
}

// Response from api calls
type Response struct {
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

// ExtractWithRules is the core function that handles extraction with or without a specific rule
func (f *UReadability) extractWithRules(ctx context.Context, reqURL string, rule *datastore.Rule) (*Response, error) {
	log.Printf("[INFO] extract %s", reqURL)
	rb := &Response{}

	// look up a rule by domain once up front (unless one was explicitly passed) so retriever
	// selection and getContent share the same lookup instead of paying for two round-trips.
	if rule == nil && f.Rules != nil {
		if r, found := f.Rules.Get(ctx, reqURL); found {
			rule = &r
		}
	}

	result, err := f.pickRetriever(rule).Retrieve(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	rb.URL = result.URL

	var body string
	rb.ContentType, rb.Charset, body = f.toUtf8(result.Body, result.Header)
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

	log.Printf("[INFO] completed for %s, url=%s", rb.Title, rb.URL)
	return rb, nil
}

// getContent retrieves content from raw body string, both content (text only) and rich (with html tags).
// if rule is provided, it tries the custom rule first and falls back to the general parser on failure.
// rule lookup for a given URL is done upstream in extractWithRules.
func (f *UReadability) getContent(_ context.Context, body, reqURL string, rule *datastore.Rule) (content, rich string, err error) {
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
		if content, rich, err = customParser(body, reqURL, *rule); err == nil {
			return content, rich, nil
		}
		log.Printf("[WARN] custom extractor failed for %s, error=%v", reqURL, err) // back to general parser
	}

	return genParser(body, reqURL)
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
