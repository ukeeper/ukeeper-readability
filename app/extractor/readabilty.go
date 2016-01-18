package extractor

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mauidude/go-readability"
	"umputun.com/ukeeper/ureadability/app/datastore"
)

//UReadability implements fetcher & exrtactor for local readbility-like functionality
type UReadability struct {
	TimeOut     time.Duration
	SnippetSize int
	Debug       bool
	Rules       datastore.Rules
}

//Response from api calls
type Response struct {
	Content   string   `json:"content"`
	Rich      string   `json:"rich_content"`
	Domain    string   `json:"domain"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Excerpt   string   `json:"excerpt"`
	Image     string   `json:"lead_image_url"`
	AllImages []string `json:"images"`
	AllLinks  []string `json:"links"`
}

var (
	reLinks  = regexp.MustCompile(`(href|src|action|background)="([^"]*)"`)
	reSpaces = regexp.MustCompile(`\s+`)
	reDot    = regexp.MustCompile(`\D(\.)\S`)
)

const userAgent = "Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/41.0.2228.0 Safari/537.36"

//Extract fetches page and retrive article
func (f UReadability) Extract(reqURL string) (rb *Response, err error) {
	log.Printf("extract %s", reqURL)
	rb = &Response{}

	httpClient := &http.Client{Timeout: time.Second * f.TimeOut}
	req, err := http.NewRequest("GET", reqURL, nil)
	req.Close = true
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)

	if err != nil {
		log.Printf("failed to get anyting from %s, error=%v", reqURL, err)
		return nil, err
	}
	defer resp.Body.Close()

	rb.URL = resp.Request.URL.String()
	dataBytes, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Printf("failed to read data from %s, error=%v", reqURL, err)
		return nil, err
	}

	body := string(dataBytes)
	rb.Content, rb.Rich, err = f.getContent(body, reqURL)
	if err != nil {
		log.Printf("failed to parse %s, error=%v", reqURL, err)
		return nil, err
	}

	dbody, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	rb.Title = dbody.Find("title").Text()

	if r, err := url.Parse(rb.URL); err == nil {
		rb.Domain = r.Host
	}

	rb.Content = f.getText(rb.Content, rb.Title)
	rb.Rich, rb.AllLinks = f.normalizeLinks(rb.Rich, resp.Request)
	rb.Excerpt = f.getSnippet(rb.Content)
	darticle, err := goquery.NewDocumentFromReader(strings.NewReader(rb.Rich))
	if im, allImgs, ok := f.extractPics(darticle.Find("img"), reqURL); ok {
		rb.Image = im
		rb.AllImages = allImgs
	}

	log.Printf("completed for %s, url=%s", rb.Title, rb.URL)
	return rb, nil
}

//gets content from raw body string
func (f UReadability) getContent(body string, reqURL string) (content string, rich string, err error) {

	//general parser
	genParser := func(body string, reqURL string) (content string, rich string, err error) {
		doc, err := readability.NewDocument(body)
		if err != nil {
			return "", "", err
		}
		content, rich = doc.Content()
		return content, rich, nil
	}

	//custom rules parser
	customParser := func(body string, reqURL string, rule datastore.Rule) (content string, rich string, err error) {
		log.Printf("custom extractor for %s", reqURL)
		dbody, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		var res string
		dbody.Find(rule.Content).Each(func(i int, s *goquery.Selection) {
			if html, err := s.Html(); err == nil {
				res += html
			}
		})
		if res == "" {
			return "", "", fmt.Errorf("nothing extracted from %s, rule=%v", reqURL, rule)
		}
		return f.getText(res, ""), res, nil
	}

	if f.Rules != nil {
		r := f.Rules
		if rule, found := r.Get(reqURL); found {
			if content, rich, err = customParser(body, reqURL, rule); err == nil {
				return content, rich, err
			}
			log.Printf("custom extractor failed for %s, error=%v", reqURL, err) //back to general parser
		}
	} else {
		log.Printf("no rules defined!")
	}

	return genParser(body, reqURL)
}

//makes all links absolute and returns all found links
func (f UReadability) normalizeLinks(data string, reqContext *http.Request) (result string, links []string) {

	absoluteLink := func(link string) (absLink string, chnaged bool) {
		if r, err := reqContext.URL.Parse(link); err == nil {
			return r.String(), r.String() != link
		}
		return "", false
	}

	result = data
	matches := reLinks.FindAllStringSubmatch(data, -1)
	normalizedCount := 0
	for _, m := range matches {
		srcLink := m[len(m)-1] //link in last element of the group
		dstLink := srcLink
		if absLink, changed := absoluteLink(srcLink); changed {
			dstLink = absLink
			srcLink := fmt.Sprintf(`"%s"`, srcLink)
			absLink = fmt.Sprintf(`"%s"`, absLink)
			result = strings.Replace(result, srcLink, absLink, -1)
			if f.Debug {
				log.Printf("normlized %s -> %s", srcLink, dstLink)
			}
			normalizedCount++
		}
		links = append(links, dstLink)
	}
	if f.Debug {
		log.Printf("normalized %d links", normalizedCount)
	}
	return result, links
}
