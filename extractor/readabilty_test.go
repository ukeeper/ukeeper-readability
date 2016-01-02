package extractor

import (
	"log"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractURL(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	t.Log("full url")
	rb, err := lr.Extract("http://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/")
	assert.Nil(t, err)
	assert.Equal(t, "http://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", rb.URL, "not changed")
	assert.Equal(t, "Всем миром для общей пользы", rb.Title)
	assert.Equal(t, 9669, len(rb.Content))

	t.Log("short url")
	rb, err = lr.Extract("http://goo.gl/IAvTHr")
	assert.Nil(t, err)
	assert.Equal(t, "http://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", rb.URL, "full url")
	assert.Equal(t, 9669, len(rb.Content))
}

func TestExtactGeneral(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	a, err := lr.Extract("http://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/")
	assert.Nil(t, err)
	assert.Equal(t, "Всем миром для общей пользы", a.Title)
	assert.Equal(t, "http://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", a.URL)
	assert.Equal(t, "5 Дек. 2015: новая версия уже тут! Спасибо всем неравнодушным, и конечно Игорю Адаменко, который придумал и реализовал весь этот UI. Всем, у кого остались замечательные идеи, рекомендую ...", a.Excerpt)
	assert.Equal(t, "p.umputun.com", a.Domain)

	a, err = lr.Extract("http://goo.gl/v48b6Q")
	assert.Nil(t, err)
	assert.Equal(t, "UWP - Выпуск 369 - Еженедельный подкаст от Umputun", a.Title)
	assert.Equal(t, "http://podcast.umputun.com/p/2015/11/22/podcast-369/", a.URL)
	assert.Equal(t, "UWP - Выпуск 369 22-11-2015 | Comments Нагло ходил в гости. Табличка на двери сработала на 50%Никогда нас школа не хвалила. Девочка осваивает новый прибор. Мое неприятие их логики. И разошлись по ...", a.Excerpt)
	assert.Equal(t, "http://podcast.umputun.com/images/uwp/uwp369.jpg", a.Image)
	assert.Equal(t, "podcast.umputun.com", a.Domain)
	assert.Equal(t, 10, len(a.AllLinks))
	assert.Equal(t, "http://podcast.umputun.com/p/2015/11/22/podcast-369/#disqus_thread", a.AllLinks[0])
	assert.Equal(t, "http://podcast.umputun.com/images/uwp/uwp369.jpg", a.AllLinks[1])
	log.Printf("links=%v", a.AllLinks)

}

func TestExtractPics(t *testing.T) {
	t.Log("test main pic")
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	a, err := lr.Extract("http://p.umputun.com/2015/09/25/poiezdka-s-apple-maps/")
	assert.Nil(t, err)
	assert.Equal(t, "http://p.umputun.com/content/images/2015/09/n891a_20150925_023343-minwz.png", a.Image)
	assert.Equal(t, 3, len(a.AllImages))
	assert.Equal(t, "http://p.umputun.com/content/images/2015/09/n891a_20150925_023343-minwz.png", a.AllImages[0])
	assert.Equal(t, "http://p.umputun.com/content/images/2015/09/ios9nav-heavytraffic-6c-1.jpg", a.AllImages[1])
	assert.Equal(t, "http://p.umputun.com/content/images/2015/09/apple-maps-app-220-100.jpg", a.AllImages[2])
}

func TestNormilizeLinks(t *testing.T) {
	inp := `blah <img src="/aaa.png"/> sdfasd <a href="/blah2/aa.link">something</a> blah33 <img src="//aaa.com/xyz.jpg">xx</img>`
	u, _ := url.Parse("http://umputun.com/blah")
	out, links := normalizeLinks(inp, &http.Request{URL: u})
	assert.Equal(t, `blah <img src="http://umputun.com/aaa.png"/> sdfasd <a href="http://umputun.com/blah2/aa.link">something</a> blah33 <img src="http://aaa.com/xyz.jpg">xx</img>`, out)
	assert.Equal(t, 3, len(links))
}

func TestNormilizeLinksIssue(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	_, err := lr.Extract("https://git-scm.com/book/en/v2/Git-Tools-Submodules")
	assert.Nil(t, err)
}
