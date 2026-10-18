package render

import (
	"bytes"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Sanitize keeps only the elements and attributes of the spec's allowlist
// (docs/specs/render.md#sanitizer): what the renderer draws, and the
// harmless part of a note's raw HTML. It never adds markup and keeps the
// order of what it keeps. Text is passed through as the tokenizer read it,
// so entities stay as written; attribute values are written escaped.
func Sanitize(h string) string {
	z := html.NewTokenizer(strings.NewReader(h))
	var out bytes.Buffer
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				return out.String()
			}
			return out.String() // A tokenizer error ends the input; what came before stands.
		case html.TextToken:
			out.Write(z.Raw())
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if dropsContent(t) {
				if tt == html.StartTagToken && !isVoid(t.Data) {
					skipTo(z, t.Data)
				}
				continue
			}
			if allowedElement(t.Data) {
				writeStart(&out, t)
			}
		case html.EndTagToken:
			t := z.Token()
			if allowedElement(t.Data) && !isVoid(t.Data) {
				out.WriteString("</" + t.Data + ">")
			}
		}
		// Comments and doctypes go.
	}
}

// skipTo consumes tokens through the end tag that closes an element named
// name, counting nested ones; to the end of input when there is none.
func skipTo(z *html.Tokenizer, name string) {
	depth := 1
	for {
		switch z.Next() {
		case html.ErrorToken:
			return
		case html.StartTagToken:
			if n, _ := z.TagName(); string(n) == name {
				depth++
			}
		case html.EndTagToken:
			if n, _ := z.TagName(); string(n) == name {
				depth--
				if depth == 0 {
					return
				}
			}
		}
	}
}

func writeStart(out *bytes.Buffer, t html.Token) {
	out.WriteString("<" + t.Data)
	for _, a := range t.Attr {
		if a.Namespace != "" || !allowedAttr(t.Data, a.Key, a.Val) {
			continue
		}
		out.WriteString(" " + a.Key + `="` + html.EscapeString(a.Val) + `"`)
	}
	out.WriteByte('>')
}

// dropsContent reports whether an element goes with everything in it:
// what runs script, loads or embeds content, submits, or styles the page,
// and an input that is not a checkbox.
func dropsContent(t html.Token) bool {
	switch t.Data {
	case "script", "style", "template", "iframe", "frame", "frameset", "object", "embed", "applet", "param",
		"form", "button", "select", "option", "optgroup", "datalist", "textarea", "link", "meta", "base",
		"svg", "math", "noscript", "noembed", "noframes", "title", "xmp", "plaintext", "head", "portal":
		return true
	case "input":
		for _, a := range t.Attr {
			if a.Key == "type" && strings.EqualFold(a.Val, "checkbox") {
				return false
			}
		}
		return true
	}
	return false
}

func isVoid(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func allowedElement(name string) bool {
	switch name {
	case "p", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "code", "em", "i",
		"strong", "b", "del", "s", "u", "ins", "mark", "sup", "sub", "small", "kbd", "abbr", "q", "cite",
		"span", "div", "ul", "ol", "li", "dl", "dt", "dd", "table", "thead", "tbody", "tfoot", "tr",
		"th", "td", "details", "summary", "figure", "figcaption", "a", "img", "audio", "video", "input":
		return true
	}
	return false
}

func allowedAttr(element, key, val string) bool {
	switch key {
	case "class", "title", "id", "role", "lang", "dir", "data-href", "data-path", "data-blob", "data-tag",
		"data-tex", "data-callout", "data-fold", "data-line", "data-block":
		return true
	}
	switch element {
	case "ol":
		return key == "start"
	case "li":
		return key == "value"
	case "th", "td":
		return key == "align" || key == "colspan" || key == "rowspan"
	case "a":
		return key == "rel" || key == "href" && safeURL(val, true)
	case "img":
		return key == "alt" || key == "width" || key == "height" || key == "src" && safeURL(val, false)
	case "audio", "video":
		return key == "controls"
	case "input":
		return key == "type" || key == "checked" || key == "disabled"
	}
	return false
}

// safeURL reports whether a URL may stay: http, https and, for a link,
// mailto, or for a link a fragment of this page. A browser ignores
// whitespace and control characters around and within a scheme
// ("java\tscript:"), so they are removed before it is read.
func safeURL(v string, link bool) bool {
	clean := strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, v)
	if link && strings.HasPrefix(clean, "#") {
		return true
	}
	colon := strings.IndexByte(clean, ':')
	if colon < 0 || strings.ContainsAny(clean[:colon], "/?#") {
		return false // relative: the app names vault files by data-blob and data-path, never by URL
	}
	switch strings.ToLower(clean[:colon]) {
	case "http", "https":
		return true
	case "mailto":
		return link
	}
	return false
}
