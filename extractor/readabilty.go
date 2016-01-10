package extractor

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mauidude/go-readability"
	"umputun.com/ukeeper/ureadability/datastore"
	"umputun.com/ukeeper/ureadability/sanitize"
)

//UReadability implements fetcher & exrtactor for local readbility-like functionality
type UReadability struct {
	TimeOut     time.Duration
	SnippetSize int
	Debug       bool
	Rules       *datastore.Rules
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

func (f UReadability) getContent(body string, reqURL string) (content string, rich string, err error) {

	genParser := func(body string, reqURL string) (content string, rich string, err error) {
		doc, err := readability.NewDocument(body)
		if err != nil {
			return "", "", err
		}
		content, rich = doc.Content()
		return content, rich, nil
	}

	customParser := func(body string, reqURL string, rule datastore.Rule) (content string, rich string, err error) {
		return "", "", nil
	}

	if f.Rules != nil {
		r := *f.Rules
		if rule, found := r.Get(reqURL); found {
			return customParser(body, reqURL, rule)
		}
	}

	return genParser(body, reqURL)
}

func (f UReadability) getText(content string, title string) string {
	cleanText := sanitize.HTML(content)
	cleanText = strings.Replace(cleanText, title, "", 1) //get rid of title in snippet
	cleanText = strings.Replace(cleanText, "\t", " ", -1)
	cleanText = strings.TrimSpace(cleanText)

	//replace multiple spaces by one space
	cleanText = reSpaces.ReplaceAllString(cleanText, " ")

	//fix joined sentences due lack of \n
	matches := reDot.FindAllStringSubmatch(cleanText, -1)
	for _, m := range matches {
		src := m[0]
		dst := strings.Replace(src, ".", ". ", 1)
		cleanText = strings.Replace(cleanText, src, dst, 1)
	}
	return cleanText
}

func (f UReadability) customParse(url string, data string, rule datastore.Rule) (rb *Response, err error) {
	return nil, fmt.Errorf("not found")
}

func (f UReadability) getSnippet(cleanText string) string {
	cleanText = strings.Replace(cleanText, "\n", " ", -1)
	size := len([]rune(cleanText))
	if size > f.SnippetSize {
		size = f.SnippetSize
	}
	snippet := []rune(cleanText)[:size]
	for i := len(snippet) - 1; i >= 0; i-- {
		if snippet[i] == ' ' {
			snippet = snippet[:i]
			break
		}
	}
	return string(snippet) + " ..."
}

func (f UReadability) extractPics(imgSelect *goquery.Selection, url string) (mainImage string, allImages []string, ok bool) {

	images := make(map[int]string)

	imgSelect.Each(func(i int, s *goquery.Selection) {
		if im, ok := s.Attr("src"); ok {
			images[f.getImageSize(im)] = im
			allImages = append(allImages, im)
		}
	})

	if len(images) == 0 {
		return "", nil, false
	}

	//get biggest picture
	keys := make([]int, 0, len(images))
	for k := range images {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	mainImage = images[keys[0]]
	f.debugf("total images from %s = %d, main=%s (%d)", url, len(images), mainImage, keys[0])
	return mainImage, allImages, true
}

func (f UReadability) getImageSize(url string) int {
	httpClient := &http.Client{Timeout: time.Second * 30}
	req, err := http.NewRequest("GET", url, nil)
	req.Close = true
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("can't get %s, error=%v", url, err)
		return 0
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("failed to get %s, err=%v", url, err)
		return 0
	}
	return len(data)
}

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
			f.debugf("%s -> %s", srcLink, dstLink)
			normalizedCount++
		}
		links = append(links, dstLink)
	}
	f.debugf("normalized %d links", normalizedCount)
	return result, links
}

func (f UReadability) debugf(format string, v ...interface{}) {
	if f.Debug {
		log.Printf(format, v)
	}
}
