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

const defaultMaxGPTIter = 3

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
	Retriever   Retriever
	AIEvaluator AIEvaluator
	MaxGPTIter  int

	defaultRetrieverOnce sync.Once
	defaultRetriever     Retriever
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
			// in normal mode, only evaluate when domain has no existing rule
			if f.Rules != nil {
				if _, found := f.Rules.Get(ctx, reqURL); !found {
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

// maxGPTIter returns MaxGPTIter or the default if not set
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

	for i := range maxIter {
		eval, err := f.AIEvaluator.Evaluate(ctx, reqURL, best.Content, htmlBody)
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

		// try the suggested selector on the HTML body
		newContent, newRich, err := f.extractWithSelector(htmlBody, eval.Selector)
		if err != nil || newContent == "" {
			log.Printf("[WARN] AI selector %q produced no content for %s: %v", eval.Selector, reqURL, err)
			continue
		}

		// rebuild the response with new content
		improved := *best
		improved.Content = f.getText(newContent, best.Title)
		improved.Rich = newRich
		improved.Excerpt = f.getSnippet(improved.Content)
		best = &improved
		bestSelector = eval.Selector
	}

	// save rule if we found a better selector
	if bestSelector != "" && f.Rules != nil {
		rule := datastore.Rule{
			Domain:  best.Domain,
			Content: bestSelector,
			Enabled: true,
			User:    "ai-evaluator",
		}
		if _, err := f.Rules.Save(ctx, rule); err != nil {
			log.Printf("[WARN] failed to save AI-suggested rule for %s: %v", best.Domain, err)
		} else {
			log.Printf("[INFO] saved AI-suggested rule for %s: %q", best.Domain, bestSelector)
		}
	}

	return best
}

// extractWithSelector applies a CSS selector to the HTML body and returns the extracted content
func (f *UReadability) extractWithSelector(htmlBody, selector string) (content, rich string, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return "", "", err
	}
	var res string
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		if html, err := s.Html(); err == nil {
			res += html
		}
	})
	if res == "" {
		return "", "", nil
	}
	return f.getText(res, ""), res, nil
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
		log.Print("[DEBUG] no rules defined!")
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
