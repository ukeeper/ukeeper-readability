package extractor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fh, err := os.Open("testdata/poiezdka-s-apple-maps.html")
		testHTML, err := io.ReadAll(fh)
		require.NoError(t, err)
		require.NoError(t, fh.Close())
		_, err = w.Write(testHTML)
		require.NoError(t, err)
	}))
	defer ts.Close()

	t.Log("test main pic")
	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200}
	a, err := lr.Extract(context.Background(), ts.URL+"/2015/09/25/poiezdka-s-apple-maps/")
	require.NoError(t, err)
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
	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200}
	t.Run("normal image retrieval", func(t *testing.T) {
		data := `<body>
		<img class="alignright size-full wp-image-944214 lazyloadableImage lazyLoad-fadeIn" alt="View Page Source" width="308" height="508" data-original="https://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg" src="https://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg"></body>`
		d, err := goquery.NewDocumentFromReader(strings.NewReader(data))
		require.NoError(t, err)
		sel := d.Find("img")
		im, allImages, ok := lr.extractPics(sel, "url")
		assert.True(t, ok)
		assert.Equal(t, 1, len(allImages))
		assert.Equal(t, "https://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg", im)
	})

	t.Run("no image", func(t *testing.T) {
		data := `<body></body>`
		d, err := goquery.NewDocumentFromReader(strings.NewReader(data))
		require.NoError(t, err)
		sel := d.Find("img")
		im, allImages, ok := lr.extractPics(sel, "url")
		assert.False(t, ok)
		assert.Empty(t, allImages)
		assert.Empty(t, im)
	})

	t.Run("bad URL", func(t *testing.T) {
		data := `<body><img src="http://bad_url"></body>`
		d, err := goquery.NewDocumentFromReader(strings.NewReader(data))
		require.NoError(t, err)
		sel := d.Find("img")
		im, allImages, ok := lr.extractPics(sel, "url")
		assert.True(t, ok)
		assert.Equal(t, 1, len(allImages))
		assert.Equal(t, "http://bad_url", im)
	})

	t.Run("bad body of the image", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1") // error on reading body
		}))
		defer ts.Close()
		data := fmt.Sprintf(`<body><img src="%s"></body>`, ts.URL)
		d, err := goquery.NewDocumentFromReader(strings.NewReader(data))
		require.NoError(t, err)
		sel := d.Find("img")
		im, allImages, ok := lr.extractPics(sel, "url")
		assert.True(t, ok)
		assert.Equal(t, 1, len(allImages))
		assert.Equal(t, ts.URL, im)
	})
}
