package rumable

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/tdewolff/parse/v2"
	htmllex "github.com/tdewolff/parse/v2/html"
)

type htmlElement struct {
	name  string
	attrs map[string]string
	text  string
}

type htmlDocument struct {
	elements []htmlElement
	head     int
}

var headAnchor = regexp.MustCompile(`(?i)<head>`)

// Inspect HTML tokens, not tag-shaped text inside scripts, comments, attributes
// or inert templates. This is a probe, not a browser execution guarantee.
func inspectHTML(body []byte) (htmlDocument, error) {
	doc := htmlDocument{head: -1}
	// The lexer normalizes names in its input; keep response bytes untouched.
	input := parse.NewInput(bytes.NewReader(body))
	lexer := htmllex.NewLexer(input)
	prelude, seenHTML := true, false
	templates, noscript, script := 0, false, -1
	for {
		start := input.Offset()
		kind, data := lexer.Next()
		switch kind {
		case htmllex.ErrorToken:
			if lexer.Err() != io.EOF {
				return doc, lexer.Err()
			}
			return doc, nil
		case htmllex.StartTagToken:
			name := string(lexer.Text())
			a := map[string]string{}
			for {
				kind, _ = lexer.Next()
				if kind != htmllex.AttributeToken {
					break
				}
				key, value := string(lexer.AttrKey()), string(lexer.AttrVal())
				if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
					value = value[1 : len(value)-1]
				}
				// HTML keeps the first duplicate attribute.
				if _, exists := a[key]; !exists {
					a[key] = html.UnescapeString(value)
				}
			}
			if kind != htmllex.StartTagCloseToken && kind != htmllex.StartTagVoidToken {
				return doc, fmt.Errorf("incomplete HTML start tag")
			}
			if prelude && name == "head" && strings.EqualFold(string(body[start:input.Offset()]), "<head>") {
				doc.head = start
			}
			if name == "html" && !seenHTML && prelude {
				seenHTML = true
			} else {
				prelude = false
			}
			if noscript {
				continue
			}
			if name == "noscript" {
				noscript = true
				continue
			}
			if name == "template" {
				templates++
				continue
			}
			if templates > 0 {
				continue
			}
			if name == "script" || name == "meta" || name == "base" {
				doc.elements = append(doc.elements, htmlElement{name: name, attrs: a})
				if name == "script" {
					script = len(doc.elements) - 1
				}
			}
		case htmllex.EndTagToken:
			name := string(lexer.Text())
			prelude = false
			if name == "noscript" {
				noscript = false
			}
			if noscript {
				continue
			}
			if name == "template" && templates > 0 {
				templates--
			}
			if name == "script" {
				script = -1
			}
		case htmllex.TextToken:
			if prelude && strings.TrimSpace(strings.TrimPrefix(string(data), "\xef\xbb\xbf")) != "" {
				prelude = false
			}
			if script >= 0 {
				doc.elements[script].text += string(data)
			}
		case htmllex.CommentToken, htmllex.DoctypeToken:
			// Comments and the doctype may precede the document head.
		default:
			prelude = false
		}
	}
}

func (d htmlDocument) checkAnchor(body []byte) error {
	first := headAnchor.FindIndex(body)
	if d.head < 0 || first == nil || first[0] != d.head {
		return fmt.Errorf("no safe literal <head> injection anchor: require a plain opening <head> before page content, with no earlier <head> text in comments or attributes")
	}
	return nil
}
