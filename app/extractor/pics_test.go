package extractor

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

func TestExtractPics(t *testing.T) {
	t.Log("test main pic")
	lr := UReadability{TimeOut: 30, SnippetSize: 200}
	a, err := lr.Extract("https://p.umputun.com/2015/09/25/poiezdka-s-apple-maps/")
	assert.Nil(t, err)
	assert.Equal(t, "https://p.umputun.com/images/posts/n891a_20150925_023343-minwz.png#floatright", a.Image)
	assert.Equal(t, 3, len(a.AllImages))
	assert.Equal(t, "https://p.umputun.com/images/posts/n891a_20150925_023343-minwz.png#floatright", a.AllImages[0])
	assert.Equal(t, "https://p.umputun.com/images/posts/ios9nav-heavytraffic-6c-1.jpg#floatleft", a.AllImages[1])
	assert.Equal(t, "https://p.umputun.com/images/posts/apple-maps-app-220-100.jpg#floatright", a.AllImages[2])
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
