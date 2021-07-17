package extractor

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"testing"
	"time"

	"gopkg.in/mgo.v2/bson"

	"ukeeper.com/ureadability/app/datastore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractURL(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	t.Log("full url")
	rb, err := lr.Extract("https://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/")
	assert.Nil(t, err)
	assert.Equal(t, "https://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", rb.URL, "not changed")
	assert.Equal(t, "Всем миром для общей пользы • Umputun тут был", rb.Title)
	assert.Equal(t, 9665, len(rb.Content))

	t.Log("short url")
	rb, err = lr.Extract("https://goo.gl/IAvTHr")
	assert.Nil(t, err)
	assert.Equal(t, "https://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", rb.URL, "full url")
	assert.Equal(t, 9665, len(rb.Content))
}

func TestExtactGeneral(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	a, err := lr.Extract("https://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/")
	assert.Nil(t, err)
	assert.Equal(t, "Всем миром для общей пользы • Umputun тут был", a.Title)
	assert.Equal(t, "https://p.umputun.com/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", a.URL)
	assert.Equal(t, "Не первый раз я практикую идею “а давайте, ребята, сделаем для общего блага …”, и вот опять. В нашем подкасте радио-т есть незаменимый инструмент, позволяющий собирать новости, готовить их к выпуску, ...", a.Excerpt)
	assert.Equal(t, "p.umputun.com", a.Domain)

	a, err = lr.Extract("http://goo.gl/v48b6Q")
	assert.Nil(t, err)
	assert.Equal(t, "UWP - Выпуск 369", a.Title)
	assert.Equal(t, "https://podcast.umputun.com/p/2015/11/22/podcast-369/", a.URL)
	assert.Equal(t, "UWP - Выпуск 369 22-11-2015 | Comments Нагло ходил в гости. Табличка на двери сработала на 50%Никогда нас школа не хвалила. Девочка осваивает новый прибор. Мое неприятие их логики. И разошлись по ...", a.Excerpt)
	assert.Equal(t, "https://podcast.umputun.com/images/uwp/uwp369.jpg", a.Image)
	assert.Equal(t, "podcast.umputun.com", a.Domain)
	require.Equal(t, 10, len(a.AllLinks))
	assert.Equal(t, "https://podcast.umputun.com/p/2015/11/22/podcast-369/#disqus_thread", a.AllLinks[0])
	assert.Equal(t, "https://podcast.umputun.com/images/uwp/uwp369.jpg", a.AllLinks[1])
	log.Printf("links=%v", a.AllLinks)

}

func TestNormilizeLinks(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	inp := `blah <img src="/aaa.png"/> sdfasd <a href="/blah2/aa.link">something</a> blah33 <img src="//aaa.com/xyz.jpg">xx</img>`
	u, _ := url.Parse("http://ukeeper.com/blah")
	out, links := lr.normalizeLinks(inp, &http.Request{URL: u})
	assert.Equal(t, `blah <img src="http://ukeeper.com/aaa.png"/> sdfasd <a href="http://ukeeper.com/blah2/aa.link">something</a> blah33 <img src="http://aaa.com/xyz.jpg">xx</img>`, out)
	assert.Equal(t, 3, len(links))

	inp = `<body>
		<img class="alignright size-full wp-image-944214 lazyloadableImage lazyLoad-fadeIn" alt="View Page Source" width="308" height="508" data-original="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg" src="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg"></body>`
	_, links = lr.normalizeLinks(inp, &http.Request{URL: u})
	assert.Equal(t, 1, len(links))
	assert.Equal(t, "http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg", links[0])

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
func (m RulesMock) GetByID(id bson.ObjectId) (datastore.Rule, bool)  { return datastore.Rule{}, false }
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
	assert.Nil(t, err)
	body := string(dataBytes)

	content, rich, err := lr.getContent(body, "http://p.umputun.com/2015/09/25/poiezdka-s-apple-maps/")
	assert.Nil(t, err)
	assert.Equal(t, 6988, len(content))
	assert.Equal(t, 7169, len(rich))
}
