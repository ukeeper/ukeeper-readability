package extractor

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	outlineMinText   = 100 // elements with less visible text are left out of the outline
	outlineMaxLines  = 120
	outlineMaxDepth  = 12
	outlineMaxClass  = 3
	outlineMaxName   = 40
	outlineSnippet   = 80
	outlineMaxThresh = 1 << 20
)

// outlineNode is an element of the page body with the size of its visible text
type outlineNode struct {
	node       *html.Node
	textLen    int
	paragraphs int
	children   []*outlineNode
}

// pageOutline describes the structure of the page body for a model: one line per element that holds
// a meaningful amount of text, with its tag, id, classes, text length and paragraph count.
// The outline is much shorter than the raw HTML and leaves out scripts, styles and markup noise.
func pageOutline(root *html.Node) string {
	body := findBody(root)
	if body == nil {
		return ""
	}
	tree := buildOutline(body)

	// raise the text threshold until the outline fits
	var lines []string
	for threshold := outlineMinText; threshold <= outlineMaxThresh; threshold *= 2 {
		lines = lines[:0]
		tree.render(&lines, 0, threshold)
		if len(lines) <= outlineMaxLines {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// visibleTextLen is the number of characters of visible text under n, whitespace collapsed
func visibleTextLen(n *html.Node) int {
	if n == nil {
		return 0
	}
	return buildOutline(n).textLen
}

func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if b := findBody(c); b != nil {
			return b
		}
	}
	return nil
}

func skipped(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Noscript, atom.Template, atom.Svg, atom.Iframe:
		return true
	}
	return false
}

func buildOutline(n *html.Node) *outlineNode {
	o := &outlineNode{node: n}
	if n.DataAtom == atom.P {
		o.paragraphs = 1
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode:
			o.textLen += len([]rune(strings.Join(strings.Fields(c.Data), " ")))
		case c.Type == html.ElementNode && !skipped(c):
			child := buildOutline(c)
			o.textLen += child.textLen
			o.paragraphs += child.paragraphs
			o.children = append(o.children, child)
		default: // comments, doctype and skipped elements carry no visible text
		}
	}
	return o
}

func (o *outlineNode) render(lines *[]string, depth, threshold int) {
	if o.textLen < threshold || depth > outlineMaxDepth {
		return
	}
	line := fmt.Sprintf("%s%s text=%d p=%d", strings.Repeat("  ", depth), o.label(), o.textLen, o.paragraphs)
	*lines = append(*lines, line)
	before := len(*lines)
	for _, c := range o.children {
		c.render(lines, depth+1, threshold)
	}
	if len(*lines) == before { // nothing below, show how the text starts
		(*lines)[before-1] += fmt.Sprintf(" %q", firstText(o.node, outlineSnippet))
	}
}

// label is tag#id.class.class, with overlong names cut
func (o *outlineNode) label() string {
	var id string
	var classes []string
	for _, a := range o.node.Attr {
		switch a.Key {
		case "id":
			id = cutName(a.Val)
		case "class":
			for _, c := range strings.Fields(a.Val) {
				if len(classes) < outlineMaxClass {
					classes = append(classes, cutName(c))
				}
			}
		default: // other attributes do not help to find the article
		}
	}
	label := o.node.Data
	if id != "" {
		label += "#" + id
	}
	if len(classes) > 0 {
		label += "." + strings.Join(classes, ".")
	}
	return label
}

func cutName(s string) string {
	s = strings.Join(strings.Fields(s), "")
	if r := []rune(s); len(r) > outlineMaxName {
		return string(r[:outlineMaxName])
	}
	return s
}

// firstText returns up to limit characters of the visible text under n
func firstText(n *html.Node, limit int) string {
	var parts []string
	var size int
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil && size < limit; c = c.NextSibling {
			switch {
			case c.Type == html.TextNode:
				if t := strings.Join(strings.Fields(c.Data), " "); t != "" {
					parts = append(parts, t)
					size += len([]rune(t)) + 1
				}
			case c.Type == html.ElementNode && !skipped(c):
				walk(c)
			default: // comments, doctype and skipped elements carry no visible text
			}
		}
	}
	walk(n)
	text := strings.Join(parts, " ")
	if r := []rune(text); len(r) > limit {
		return string(r[:limit])
	}
	return text
}
