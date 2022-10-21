package extractor

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/kennygrant/sanitize"
	"golang.org/x/net/html/charset"
)

// get clean text from html content
func (f UReadability) getText(content, title string) string {
	cleanText := sanitize.HTML(content)
	cleanText = strings.Replace(cleanText, title, "", 1) // get rid of title in snippet
	cleanText = strings.ReplaceAll(cleanText, "\t", " ")
	cleanText = strings.TrimSpace(cleanText)

	// replace multiple spaces by one space
	cleanText = reSpaces.ReplaceAllString(cleanText, " ")

	// fix joined sentences due lack of \n
	matches := reDot.FindAllStringSubmatch(cleanText, -1)
	for _, m := range matches {
		src := m[0]
		dst := strings.Replace(src, ".", ". ", 1)
		cleanText = strings.Replace(cleanText, src, dst, 1)
	}
	return cleanText
}

// get snippet from clean text content
func (f UReadability) getSnippet(cleanText string) string {
	cleanText = strings.ReplaceAll(cleanText, "\n", " ")
	size := len([]rune(cleanText))
	if size > f.SnippetSize {
		size = f.SnippetSize
	}
	snippet := []rune(cleanText)[:size]
	// go back in snippet and found first space
	for i := len(snippet) - 1; i >= 0; i-- {
		if snippet[i] == ' ' {
			snippet = snippet[:i]
			break
		}
	}
	return string(snippet) + " ..."
}

// detect encoding, content type and convert content to utf8
func (f UReadability) toUtf8(content []byte, header http.Header) (contentType, origEncoding, result string) {
	getContentTypeAndEncoding := func(str string) (contentType, encoding string) { // from "text/html; charset=windows-1251"
		elems := strings.Split(str, ";")
		contentType = strings.TrimSpace(elems[0])
		if len(elems) > 1 && strings.Contains(elems[1], "charset=") {
			encoding = strings.TrimPrefix(strings.TrimSpace(elems[1]), "charset=")
		}
		return contentType, encoding
	}

	body := string(content)

	result = body
	contentType = "text/html"
	origEncoding = "utf-8"

	if h := header.Get("Content-Type"); h != "" {
		contentType, origEncoding = getContentTypeAndEncoding(h)
	}

	dbody, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return contentType, origEncoding, result
	}

	dbody.Find("head meta").Each(func(i int, s *goquery.Selection) {
		if strings.EqualFold(s.AttrOr("http-equiv", ""), "Content-Type") {
			contentTypeStr := s.AttrOr("content", "")
			contentType, origEncoding = getContentTypeAndEncoding(contentTypeStr)
		}
	})

	if origEncoding != "utf-8" {
		log.Printf("[DEBUG] non utf8 encoding detected, %s", origEncoding)
		rr, err := charset.NewReader(strings.NewReader(body), origEncoding)
		if err != nil {
			log.Printf("[WARN] charset reader failed, %v", err)
			return contentType, origEncoding, result
		}
		conv2utf8, err := io.ReadAll(rr)
		if err != nil {
			log.Printf("[WARN] convert to utf-8 failed, %b", err)
			return contentType, origEncoding, result
		}
		result = string(conv2utf8)
	}

	return contentType, origEncoding, result
}
