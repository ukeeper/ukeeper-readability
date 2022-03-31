package extractor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

func TestExtractPics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fh, err := os.Open("testdata/poiezdka-s-apple-maps.html")
		testHTML, err := io.ReadAll(fh)
		assert.NoError(t, err)
		assert.NoError(t, fh.Close())
		_, err = w.Write(testHTML)
		assert.NoError(t, err)
	}))
	defer ts.Close()

	t.Log("test main pic")
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	a, err := lr.Extract(context.Background(), ts.URL+"/2015/09/25/poiezdka-s-apple-maps/")
	assert.Nil(t, err)
	allImages := []string{
		ts.URL + "/images/posts/apple-maps-app-220-100.jpg#floatright",
		ts.URL + "/images/posts/ios9nav-heavytraffic-6c-1.jpg#floatleft",
		ts.URL + "/images/posts/n891a_20150925_023343-minwz.png#floatright",
	}
	assert.Contains(t, allImages, a.Image, "should pick one of two images as an article image")
	assert.Equal(t, allImages, a.AllImages)
}

func TestExtractPicsDirectly(t *testing.T) {
	t.Log("test pic directly")
	data := `<body>
		<img class="alignright size-full wp-image-944214 lazyloadableImage lazyLoad-fadeIn" alt="View Page Source" width="308" height="508" data-original="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg" src="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg"></body>`
	d, err := goquery.NewDocumentFromReader(strings.NewReader(data))
	assert.Nil(t, err)
	sel := d.Find("img")
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	im, allImages, ok := lr.extractPics(sel, "url")
	assert.True(t, ok)
	assert.Equal(t, 1, len(allImages))
	assert.Equal(t, "http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg", im)
}
