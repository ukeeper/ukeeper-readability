package extractor

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/ukeeper/ukeeper-redabilty/backend/datastore"
)

func TestExtractURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/IAvTHr" {
			http.Redirect(w, r, "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", http.StatusFound)
			return
		}
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			w.Header().Set("Content-Type", "text/html; charset=windows-1251") // test non-standard charset decoding
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
		if r.URL.String() == "/bad_body" {
			w.Header().Set("Content-Length", "1") // error on reading body
			return
		}
	}))
	defer ts.Close()

	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	t.Log("full url")
	rb, err := lr.Extract(context.Background(), ts.URL+"/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/")
	assert.NoError(t, err)
	assert.Equal(t, ts.URL+"/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", rb.URL, "not changed")
	assert.Equal(t, "Всем миром для общей пользы • Umputun тут был", rb.Title)
	assert.Equal(t, 9665, len(rb.Content))

	t.Log("short url")
	rb, err = lr.Extract(context.Background(), ts.URL+"/IAvTHr")
	assert.NoError(t, err)
	assert.Equal(t, ts.URL+"/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", rb.URL, "full url")
	assert.Equal(t, 9665, len(rb.Content))

	t.Log("bad body")
	rb, err = lr.Extract(context.Background(), ts.URL+"/bad_body")
	//assert.Error(t, err) // TODO: uncomment, wtf?! this should return error!
	assert.Nil(t, rb)

	t.Log("bad url")
	rb, err = lr.Extract(context.Background(), "http://bad_url")
	assert.Error(t, err)
	assert.Nil(t, rb)
}

func TestExtractGeneral(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/v48b6Q" {
			http.Redirect(w, r, "/p/2015/11/22/podcast-369/", http.StatusFound)
			return
		}
		if r.URL.String() == "/p/2015/11/22/podcast-369/" {
			fh, err := os.Open("testdata/podcast-369.html")
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
	}))
	defer ts.Close()

	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	a, err := lr.Extract(context.Background(), ts.URL+"/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/")
	assert.NoError(t, err)
	assert.Equal(t, "Всем миром для общей пользы • Umputun тут был", a.Title)
	assert.Equal(t, ts.URL+"/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", a.URL)
	assert.Equal(t, "Не первый раз я практикую идею “а давайте, ребята, сделаем для общего блага …”, и вот опять. В нашем подкасте радио-т есть незаменимый инструмент, позволяющий собирать новости, готовить их к выпуску, ...", a.Excerpt)
	assert.Contains(t, ts.URL, a.Domain)

	a, err = lr.Extract(context.Background(), ts.URL+"/v48b6Q")
	assert.NoError(t, err)
	assert.Equal(t, "UWP - Выпуск 369", a.Title)
	assert.Equal(t, ts.URL+"/p/2015/11/22/podcast-369/", a.URL)
	assert.Equal(t, "2015-11-22 Нагло ходил в гости. Табличка на двери сработала на 50%Никогда нас школа не хвалила. Девочка осваивает новый прибор. Мое неприятие их логики. И разошлись по будкам …Отбиваюсь от опасных ...", a.Excerpt)
	assert.Equal(t, "https://podcast.umputun.com/images/uwp/uwp369.jpg", a.Image)
	assert.Contains(t, ts.URL, a.Domain)
	assert.Equal(t, 13, len(a.AllLinks))
	assert.Contains(t, a.AllLinks, "https://podcast.umputun.com/media/ump_podcast369.mp3")
	assert.Contains(t, a.AllLinks, "https://podcast.umputun.com/images/uwp/uwp369.jpg")
	log.Printf("links=%v", a.AllLinks)
}

func TestNormalizeLinks(t *testing.T) {
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

func TestNormalizeLinksIssue(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	_, err := lr.Extract(context.Background(), "https://git-scm.com/book/en/v2/Git-Tools-Submodules")
	assert.NoError(t, err)
}

type RulesMock struct{}

func (m RulesMock) Get(_ context.Context, _ string) (datastore.Rule, bool) {
	return datastore.Rule{Content: "#content p, .post-title"}, true
}
func (m RulesMock) GetByID(_ context.Context, _ primitive.ObjectID) (datastore.Rule, bool) {
	return datastore.Rule{}, false
}
func (m RulesMock) Save(_ context.Context, _ datastore.Rule) (datastore.Rule, error) {
	return datastore.Rule{}, nil
}
func (m RulesMock) Disable(_ context.Context, _ primitive.ObjectID) error { return nil }
func (m RulesMock) All(_ context.Context) []datastore.Rule                { return make([]datastore.Rule, 0) }

func TestGetContentCustom(t *testing.T) {
	lr := UReadability{TimeOut: 30, SnippetSize: 200, Rules: RulesMock{}}
	httpClient := &http.Client{Timeout: time.Second * 30}
	resp, err := httpClient.Get("https://p.umputun.com/2015/09/25/poiezdka-s-apple-maps/")
	assert.NoError(t, err)
	defer resp.Body.Close()
	dataBytes, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	body := string(dataBytes)

	content, rich, err := lr.getContent(context.Background(), body, "https://p.umputun.com/2015/09/25/poiezdka-s-apple-maps/")
	assert.NoError(t, err)
	assert.Equal(t, 6988, len(content))
	assert.Equal(t, 7169, len(rich))
}
