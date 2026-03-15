package extractor

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetText(t *testing.T) {
	lr := UReadability{SnippetSize: 200}

	tests := []struct {
		name    string
		content string
		title   string
		want    string
	}{
		{name: "simple html", content: "<p>hello world</p>", title: "", want: "hello world"},
		{name: "removes title", content: "<p>My Title some text</p>", title: "My Title", want: "some text"},
		{name: "collapses whitespace", content: "<p>hello    world</p>", title: "", want: "hello world"},
		{name: "trims tabs", content: "<p>\thello\tworld</p>", title: "", want: "hello world"},
		{name: "fixes joined sentences", content: "<p>first sentence.Second sentence</p>", title: "", want: "first sentence. Second sentence"},
		{name: "empty content", content: "", title: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lr.getText(tt.content, tt.title)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetSnippet(t *testing.T) {
	lr := UReadability{SnippetSize: 20}

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "short text", text: "hello", want: "hello ..."},
		{name: "truncates at word boundary", text: "hello world this is a long text", want: "hello world this is ..."},
		{name: "replaces newlines", text: "hello\nworld this is longer text", want: "hello world this is ..."},
		{name: "empty", text: "", want: " ..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lr.getSnippet(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToUtf8(t *testing.T) {
	lr := UReadability{}

	t.Run("utf8 content", func(t *testing.T) {
		ct, enc, body := lr.toUtf8([]byte("<html><body>hello</body></html>"), http.Header{})
		assert.Equal(t, "text/html", ct)
		assert.Equal(t, "utf-8", enc)
		assert.Contains(t, body, "hello")
	})

	t.Run("content type from header", func(t *testing.T) {
		h := http.Header{}
		h.Set("Content-Type", "text/html; charset=utf-8")
		ct, enc, body := lr.toUtf8([]byte("<html><body>hello</body></html>"), h)
		assert.Equal(t, "text/html", ct)
		assert.Equal(t, "utf-8", enc)
		assert.Contains(t, body, "hello")
	})

	t.Run("encoding from meta tag", func(t *testing.T) {
		html := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1251"></head><body>hello</body></html>`
		ct, enc, _ := lr.toUtf8([]byte(html), http.Header{})
		assert.Equal(t, "text/html", ct)
		assert.Equal(t, "windows-1251", enc)
	})

	t.Run("non-utf8 conversion", func(t *testing.T) {
		h := http.Header{}
		h.Set("Content-Type", "text/html; charset=windows-1251")
		ct, enc, _ := lr.toUtf8([]byte("<html><body>hello</body></html>"), h)
		assert.Equal(t, "text/html", ct)
		assert.Equal(t, "windows-1251", enc)
	})

	t.Run("unknown charset falls back", func(t *testing.T) {
		h := http.Header{}
		h.Set("Content-Type", "text/html; charset=unknown-xyz")
		ct, enc, body := lr.toUtf8([]byte("<html><body>hello</body></html>"), h)
		assert.Equal(t, "text/html", ct)
		assert.Equal(t, "unknown-xyz", enc)
		assert.Contains(t, body, "hello")
	})
}
