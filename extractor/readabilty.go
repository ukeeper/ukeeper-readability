package fetcher

import (
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mauidude/go-readability"
	"umputun.com/ureadability/sanitize"
)

//Readability implements fetcher interface for local readbility-like functionality
type Readability struct {
	TimeOut     time.Duration
	SnippetSize int
}

type Response struct {
	Content, Rich string
	Domain        string
	URL           string
	Title         string
	Excerpt       string
	Image         string
}

func (f Readability) Extract(url string) (rb *Response, err error) {
	rb = &Response{}
	httpClient := &http.Client{Timeout: time.Second * f.TimeOut}
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Printf("failed to get anyting from %s, error=%v", url, err)
		return nil, err
	}

	rb.URL = resp.Request.URL.String()
	dataBytes, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Printf("failed to read data from %s, error=%v", url, err)
		return nil, err
	}

	body := string(dataBytes)
	doc, err := readability.NewDocument(body)
	if err != nil {
		log.Printf("failed to parse %s, error=%v", url, err)
		return nil, err
	}
	dbody, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	rb.Title = dbody.Find("title").Text()

	rb.Content, rb.Rich = doc.Content()
	rb.Rich = normalizeLinks(rb.Rich, resp.Request)
	rb.Excerpt = f.getSnippet(rb.Content, rb.Title)
	darticle, err := goquery.NewDocumentFromReader(strings.NewReader(rb.Rich))
	if im, ok := getMainPic(darticle.Find("img"), url); ok {
		rb.Image = im
	}

	return rb, nil
}

func (f Readability) getSnippet(content string, title string) string {
	cleanText := sanitize.HTML(content)
	cleanText = strings.Replace(cleanText, title, "", 1) //get rid of title in snippet
	cleanText = strings.Replace(cleanText, "\t", " ", -1)
	cleanText = strings.Replace(cleanText, "\n", " ", -1)
	cleanText = strings.TrimSpace(cleanText)

	//replace multiple spaces by one space
	re := regexp.MustCompile(`\s+`)
	cleanText = re.ReplaceAllString(cleanText, " ")

	re = regexp.MustCompile(`\D(\.)\S`)
	matches := re.FindAllStringSubmatch(cleanText, -1)
	for _, m := range matches {
		src := m[0]
		dst := strings.Replace(src, ".", ". ", 1)
		cleanText = strings.Replace(cleanText, src, dst, 1)
	}

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

func getMainPic(imgSelect *goquery.Selection, url string) (image string, ok bool) {

	images := make(map[string]int)

	imgSelect.Each(func(i int, s *goquery.Selection) {
		if im, ok := s.Attr("src"); ok {
			images[im] = getImageSize(im)
		}
	})

	//get biggest picture
	max, r := 0, ""
	for k, v := range images {
		if v > max {
			max, r = v, k
		}
	}

	if max == 0 {
		return "", false
	}

	log.Printf("total images from %s = %d, main=%s (%d)", url, len(images), r, max)
	return r, true
}

func getImageSize(url string) int {
	httpClient := &http.Client{Timeout: time.Second * 30}
	req, err := http.NewRequest("GET", url, nil)
	req.Close = true
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

func normalizeLinks(data string, reqContext *http.Request) string {

	absoluteLink := func(link string) (string, bool) {
		if r, err := reqContext.URL.Parse(link); err == nil {
			return r.String(), true
		}
		return "", false
	}

	result := data
	re := regexp.MustCompile(`(href|src|action|background)="([^"]*)"`)
	matches := re.FindAllStringSubmatch(data, -1)
	normalizedCount := 0
	for _, m := range matches {
		srcLink := m[len(m)-1] //link in last element of the group
		if dstLink, changed := absoluteLink(m[len(m)-1]); changed {
			result = strings.Replace(result, srcLink, dstLink, -1)
			normalizedCount++
			// log.Printf("normalize %s -> %s", srcLink, dstLink)
		}

	}
	log.Printf("normalized %d links", normalizedCount)
	return result
}
