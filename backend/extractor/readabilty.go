// Package extractor uses altered version of go-readabilty and local rules to get articles
package extractor

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
	"github.com/mauidude/go-readability"

	"github.com/ukeeper/ukeeper-redabilty/backend/datastore"
)

// UReadability implements fetcher & extractor for local readability-like functionality
type UReadability struct {
	TimeOut     time.Duration
	SnippetSize int
	Debug       bool
	Rules       datastore.Rules
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

const userAgent = "Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/41.0.2228.0 Safari/537.36"

// Extract fetches page and retrieves article
func (f UReadability) Extract(reqURL string) (rb *Response, err error) {
	log.Printf("[INFO] extract %s", reqURL)
	rb = &Response{}

	httpClient := &http.Client{Timeout: time.Second * f.TimeOut}
	req, err := http.NewRequest("GET", reqURL, nil)
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
	defer func() { err = resp.Body.Close() }()

	rb.URL = resp.Request.URL.String()
	dataBytes, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Printf("[WARN] failed to read data from %s, error=%v", reqURL, err)
		return nil, err
	}

	var body string
	rb.ContentType, rb.Charset, body = f.toUtf8(dataBytes, resp.Header)
	rb.Content, rb.Rich, err = f.getContent(body, reqURL)
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

// gets content from raw body string, both content (text only) and rich (with html tags)
func (f UReadability) getContent(body, reqURL string) (content, rich string, err error) {
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
		dbody.Find(rule.Content).Each(func(i int, s *goquery.Selection) {
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

	if f.Rules != nil {
		r := f.Rules
		if rule, found := r.Get(reqURL); found {
			if content, rich, err = customParser(body, reqURL, rule); err == nil {
				return content, rich, err
			}
			log.Printf("[WARN] custom extractor failed for %s, error=%v", reqURL, err) // back to general parser
		}
	} else {
		log.Printf("[DEBUG] no rules defined!")
	}

	return genParser(body, reqURL)
}

// makes all links absolute and returns all found links
func (f UReadability) normalizeLinks(data string, reqContext *http.Request) (result string, links []string) {
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
			srcLink = fmt.Sprintf(`"%s"`, srcLink)
			absLink = fmt.Sprintf(`"%s"`, absLink)
			result = strings.ReplaceAll(result, srcLink, absLink)
			if f.Debug {
				log.Printf("[DEBUG] normalized %s -> %s", srcLink, dstLink)
			}
			normalizedCount++
		}
		links = append(links, dstLink)
	}
	if f.Debug {
		log.Printf("[DEBUG] normalized %d links", normalizedCount)
	}
	return result, links
}
