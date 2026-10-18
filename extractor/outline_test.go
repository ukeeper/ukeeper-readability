package extractor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func outlineOf(t *testing.T, page string) string {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	require.NoError(t, err)
	return pageOutline(doc.Nodes[0])
}

func TestPageOutline(t *testing.T) {
	long := strings.Repeat("word ", 40) // 199 characters once the trailing space is collapsed
	short := strings.Repeat("word ", 15)

	t.Run("shows structure, sizes and the start of the text", func(t *testing.T) {
		page := `<html><head><title>t</title><script>var secret = 1;</script></head><body>
<nav class="menu"><a>Home</a></nav>
<div id="main" class="post wide">
<script>var inline = 2;</script><style>.a{}</style>
<article class="body"><p>First paragraph ` + short + `</p><p>Second ` + short + `</p></article>
</div></body></html>`
		out := outlineOf(t, page)
		lines := strings.Split(out, "\n")
		require.Len(t, lines, 3, out)
		assert.Regexp(t, `^body text=\d+ p=2$`, lines[0])
		assert.Regexp(t, `^  div#main\.post\.wide text=\d+ p=2$`, lines[1])
		assert.Regexp(t, `^    article\.body text=\d+ p=2 "First paragraph word word`, lines[2])
		assert.NotContains(t, out, "nav", "elements with little text are left out")
		assert.NotContains(t, out, "secret")
		assert.NotContains(t, out, "inline")
	})

	t.Run("leaves out scripts, styles and svg when counting text", func(t *testing.T) {
		page := `<html><body><div id="a"><p>` + long + `</p><script>` + strings.Repeat("x", 5000) + `</script>` +
			`<svg><text>` + strings.Repeat("y", 5000) + `</text></svg></div></body></html>`
		assert.Regexp(t, `div#a text=199 p=1`, outlineOf(t, page))
	})

	t.Run("long names are cut and at most three classes are shown", func(t *testing.T) {
		page := `<html><body><div id="` + strings.Repeat("i", 100) + `" class="a b c d ` + strings.Repeat("c", 100) + `">` + long + `</div></body></html>`
		out := outlineOf(t, page)
		assert.Contains(t, out, "div#"+strings.Repeat("i", 40)+".a.b.c ")
		assert.NotContains(t, out, ".d")
	})

	t.Run("raises the threshold until the outline fits", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString("<html><body><main>")
		for i := range 300 {
			fmt.Fprintf(&sb, `<section id="s%d">%s</section>`, i, strings.Repeat("a", 150))
		}
		sb.WriteString(`<section id="big">` + strings.Repeat("b", 5000) + `</section></main></body></html>`)
		out := outlineOf(t, sb.String())
		assert.LessOrEqual(t, len(strings.Split(out, "\n")), outlineMaxLines)
		assert.Contains(t, out, "section#big", "the section holding most of the text survives")
		assert.NotContains(t, out, "section#s1 ")
	})

	t.Run("no body gives nothing to show", func(t *testing.T) {
		assert.Empty(t, pageOutline(&html.Node{Type: html.DocumentNode}))
	})
}
