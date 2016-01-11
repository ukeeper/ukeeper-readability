package extractor

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"gopkg.in/mgo.v2/bson"

	"umputun.com/ukeeper/ureadability/datastore"

	"github.com/PuerkitoBio/goquery"
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

func TestExtracPicsDirectly(t *testing.T) {
	t.Log("test pic directly")
	data := `<body>
		<img class="alignright size-full wp-image-944214 lazyloadableImage lazyLoad-fadeIn" alt="View Page Source" width="308" height="508" data-original="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg" src="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg"></body>`
	d, err := goquery.NewDocumentFromReader(strings.NewReader(data))
	assert.Nil(t, err)
	sel := d.Find("img")
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	im, allImgs, ok := lr.extractPics(sel, "url")
	assert.True(t, ok)
	assert.Equal(t, 1, len(allImgs))
	assert.Equal(t, "http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg", im)
}

func TestNormilizeLinks(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	inp := `blah <img src="/aaa.png"/> sdfasd <a href="/blah2/aa.link">something</a> blah33 <img src="//aaa.com/xyz.jpg">xx</img>`
	u, _ := url.Parse("http://umputun.com/blah")
	out, links := lr.normalizeLinks(inp, &http.Request{URL: u})
	assert.Equal(t, `blah <img src="http://umputun.com/aaa.png"/> sdfasd <a href="http://umputun.com/blah2/aa.link">something</a> blah33 <img src="http://aaa.com/xyz.jpg">xx</img>`, out)
	assert.Equal(t, 3, len(links))
}

func TestNormilizeLinksIssue(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	_, err := lr.Extract("https://git-scm.com/book/en/v2/Git-Tools-Submodules")
	assert.Nil(t, err)
}

type RulesMock struct{}

func (m RulesMock) Get(rURL string) (datastore.Rule, bool) {
	return datastore.Rule{Content: "#content p, .post-title"}, true
}
func (m RulesMock) Save(rule datastore.Rule) (datastore.Rule, error) { return datastore.Rule{}, nil }
func (m RulesMock) Disable(id bson.ObjectId) error                   { return nil }
func (m RulesMock) All() []datastore.Rule                            { return make([]datastore.Rule, 0) }

func TestGetContentCustom(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200, Rules: RulesMock{}}
	httpClient := &http.Client{Timeout: time.Second * 30}
	resp, err := httpClient.Get("http://p.umputun.com/2015/09/25/poiezdka-s-apple-maps/")
	assert.Nil(t, err)
	defer resp.Body.Close()
	dataBytes, err := ioutil.ReadAll(resp.Body)
	body := string(dataBytes)

	content, rich, err := lr.getContent(body, "http://p.umputun.com/2015/09/25/poiezdka-s-apple-maps/")
	assert.Nil(t, err)
	assert.Equal(t, 6872, len(content))
	assert.Equal(t, 6904, len(rich))
}
