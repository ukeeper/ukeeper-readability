package extractor

import (
	"strings"

	"umputun.com/ukeeper/ureadability/sanitize"
)

//get clean text from html content
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

//get snippet from clean text content
func (f UReadability) getSnippet(cleanText string) string {
	cleanText = strings.Replace(cleanText, "\n", " ", -1)
	size := len([]rune(cleanText))
	if size > f.SnippetSize {
		size = f.SnippetSize
	}
	snippet := []rune(cleanText)[:size]
	//go back in snippet and found first space
	for i := len(snippet) - 1; i >= 0; i-- {
		if snippet[i] == ' ' {
			snippet = snippet[:i]
			break
		}
	}
	return string(snippet) + " ..."
}
