package rumable

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestHTMLInjectionAnchor(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		safe       bool
	}{
		{"script closing head", `<html><head><script>window.template="</head>";window.businessRan=true;</script></head></html>`, true},
		{"mixed case", "\xef\xbb\xbf<!doctype html>\n<!-- welcome -->\n<HTML lang='zh'><HeAd></HeAd></HTML>", true},
		{"fake comment first", `<!-- <head> --><html><head></head></html>`, false},
		{"fake attribute first", `<html title="<head>"><head></head></html>`, false},
		{"missing head", `<html><body><script>const x="<head>";</script></body></html>`, false},
		{"attributed head", `<html><head lang="en"><script>const x="<head>";</script></head></html>`, true},
		{"space before close", `<html><head ></head></html>`, true},
		{"quoted greater-than", `<html><head data-value='a>b' title="fixture"></head></html>`, true},
		{"multiline", "<html><head\n data-fixture='yes'\t></head></html>", true},
		{"attributed comment first", `<!-- <HEAD lang="EN"> --><html><head lang="en"></head></html>`, false},
		{"attributed attribute first", `<html title='<head lang="en">'><head lang="en"></head></html>`, false},
		{"different comment", `<!-- <head> --><html><head lang="en"></head></html>`, true},
		{"self closing", `<html><head/></html>`, false},
		{"nginx variable", `<html><head data-value="$host"></head></html>`, false},
		{"oversized", `<html><head data-value="` + strings.Repeat("x", 1024) + `"></head></html>`, false},
		{"script before head", `<script>const x=1;</script><head></head>`, false},
		{"template head", `<template><head></head></template>`, false},
		{"text before head", `not a document<head></head>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			doc, err := inspectHTML(body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, []byte(tc.body)) {
				t.Fatal("HTML inspection mutated the response")
			}
			if err = doc.checkAnchor(body); (err == nil) != tc.safe {
				t.Fatalf("safe=%v: %v", tc.safe, err)
			}
		})
	}
}

func TestInjectionProbeIgnoresInertScriptText(t *testing.T) {
	s := testSite()
	n, _, err := render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	tag := scriptTag(s, n.Script)
	fake := strings.TrimSuffix(tag, "</script>")
	for _, body := range []string{
		`<script>window.template='` + fake + `';</script>`,
		`<!-- ` + tag + ` -->`,
		`<textarea>` + tag + `</textarea>`,
		`<title>` + tag + `</title>`,
		`<style>` + tag + `</style>`,
		`<template><template>` + tag + `</template></template>`,
		`<noscript>` + tag + `</noscript>`,
		`<div title='` + tag + `'></div>`,
	} {
		p := Page{Header: http.Header{"Content-Type": {"text/html"}}, Body: []byte(`<html><head></head><body>` + body + `</body></html>`)}
		if _, err := inspectInjection(s, p, &n); err == nil {
			t.Fatalf("accepted inert injection: %s", body)
		}
		if _, err := inspectInjection(s, p, &Snapshot{}); err != nil {
			t.Fatalf("inert tag prevented disable: %v", err)
		}
		p.Body = []byte(`<html><head>` + tag + `</head><body>` + body + `</body></html>`)
		if _, err := inspectInjection(s, p, &n); err != nil {
			t.Fatalf("fake duplicate hid real injection: %v (%s)", err, body)
		}
	}
}

func TestHTMLAttributesAndPolicies(t *testing.T) {
	s := testSite()
	n, _, err := render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	// '>' inside a quoted attribute must not end the tag; first duplicate wins.
	body := `<html><head><SCRIPT title='>' DATA-RUMABLE=test data-rumable=other SRC="` + n.Script + `"></SCRIPT></head></html>`
	p := Page{Header: http.Header{"Content-Type": {"text/html"}}, Body: []byte(body)}
	if _, err := inspectInjection(s, p, &n); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{
		`<base href="https://other.example/">`,
		`<meta title='>' http-equiv=Content-Security-Policy content="script-src 'none'">`,
	} {
		p.Body = []byte(`<html><head>` + content + `</head></html>`)
		if checkCSP(s, p) == nil {
			t.Fatal("missed real base/CSP", content)
		}
		p.Body = []byte(`<html><head><!--` + content + `--><script>const example=` + "`" + content + "`" + `;</script></head></html>`)
		if err := checkCSP(s, p); err != nil {
			t.Fatal("parsed base/CSP from inert text", err)
		}
	}
}

func TestHeadAnchorIISBoundary(t *testing.T) {
	s := testSite()
	s.Nginx = nil
	s.IIS = &IIS{}
	p := Page{Header: http.Header{"Content-Type": {"text/html"}}, Body: []byte(`<html><head lang="en"></head></html>`)}
	if err := checkPage(s, p); err == nil || !strings.Contains(err.Error(), "IIS requires") {
		t.Fatalf("IIS boundary: %v", err)
	}
}

func TestHeadAnchorPreservation(t *testing.T) {
	s := testSite()
	n, _ := legacyHeadSnapshot(t, t.TempDir(), s, map[string]string{"/": `<head lang="en">`})
	for _, mode := range []struct{ enabled, preserve bool }{{false, false}, {true, true}} {
		next, _, err := renderDeploymentSDK(t.TempDir(), s, mode.enabled, &n, mode.preserve, nil)
		if err != nil {
			t.Fatal(err)
		}
		if next.HeadAnchors["/"] != n.HeadAnchors["/"] {
			t.Fatal("disable/OTel lost anchors")
		}
	}
}

func TestPlainHeadPreflight(t *testing.T) {
	for _, head := range []string{"<head>", "<HEAD>", "<HeAd>", "<head >", "<head\n>", "<head\t>", `<head data-check="yes">`, `<head lang="en">`} {
		p := Page{URL: "http://site.invalid/", Header: http.Header{"Content-Type": {"text/html"}}, Body: []byte("<html>" + head + "</head></html>")}
		err := checkPage(testSite(), p)
		if (err == nil) != strings.EqualFold(head, "<head>") {
			t.Fatalf("%s: %v", head, err)
		}
	}
}
