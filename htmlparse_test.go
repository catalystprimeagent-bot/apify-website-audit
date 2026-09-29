package main

import (
	"net/url"
	"testing"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return u
}

func TestParsePage_TitleAndMetaDescription(t *testing.T) {
	body := `<html><head><title>  Hello World  </title>
		<meta name="description" content="a twelve char"></head><body></body></html>`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/"))
	if pd.title != "Hello World" {
		t.Errorf("title = %q, want %q", pd.title, "Hello World")
	}
	if pd.metaDescriptionLength != len("a twelve char") {
		t.Errorf("metaDescriptionLength = %d, want %d", pd.metaDescriptionLength, len("a twelve char"))
	}
}

func TestParsePage_ImagesMissingAlt(t *testing.T) {
	body := `<img src="a.png" alt="a cat"><img src="b.png" alt=""><img src="c.png">`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/"))
	if pd.imagesMissingAlt != 2 {
		t.Errorf("imagesMissingAlt = %d, want 2", pd.imagesMissingAlt)
	}
}

func TestParsePage_StructuredDataViaLdJson(t *testing.T) {
	body := `<script type="application/ld+json">{"@type":"Organization"}</script>`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/"))
	if !pd.hasStructuredData {
		t.Errorf("expected hasStructuredData=true for a non-empty ld+json script")
	}
}

func TestParsePage_StructuredDataViaMicrodata(t *testing.T) {
	body := `<div itemscope itemtype="https://schema.org/Product"></div>`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/"))
	if !pd.hasStructuredData {
		t.Errorf("expected hasStructuredData=true for an itemscope element")
	}
}

func TestParsePage_NoStructuredData(t *testing.T) {
	body := `<p>just text</p>`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/"))
	if pd.hasStructuredData {
		t.Errorf("expected hasStructuredData=false with no ld+json or microdata")
	}
}

func TestParsePage_MixedContentOnHTTPSPage(t *testing.T) {
	body := `<img src="http://insecure.example.com/x.png">`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/"))
	if !pd.mixedContentFound {
		t.Errorf("expected mixedContentFound=true for an http:// image on an https page")
	}
}

func TestParsePage_ProtocolRelativeIsNotMixedContent(t *testing.T) {
	body := `<script src="//cdn.example.com/x.js"></script>`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/"))
	if pd.mixedContentFound {
		t.Errorf("expected mixedContentFound=false for a protocol-relative resource")
	}
}

func TestParsePage_MixedContentIgnoredOnHTTPPage(t *testing.T) {
	body := `<img src="http://insecure.example.com/x.png">`
	pd := parsePage([]byte(body), mustParseURL(t, "http://example.com/"))
	if pd.mixedContentFound {
		t.Errorf("mixed content only applies to https pages")
	}
}

func TestParsePage_LinksResolvedAbsoluteAndDeduped(t *testing.T) {
	body := `<a href="/a">a</a><a href="/a">a again</a><a href="https://other.com/b">b</a>` +
		`<a href="#frag">skip</a><a href="mailto:x@example.com">skip</a>`
	pd := parsePage([]byte(body), mustParseURL(t, "https://example.com/dir/"))
	want := map[string]bool{"https://example.com/a": true, "https://other.com/b": true}
	if len(pd.links) != 2 {
		t.Fatalf("expected 2 unique links, got %v", pd.links)
	}
	for _, l := range pd.links {
		if !want[l] {
			t.Errorf("unexpected link %q", l)
		}
	}
}

func TestSameSite(t *testing.T) {
	cases := []struct{ a, b string; want bool }{
		{"example.com", "example.com", true},
		{"www.example.com", "example.com", true},
		{"example.com", "other.com", false},
	}
	for _, c := range cases {
		if got := sameSite(c.a, c.b); got != c.want {
			t.Errorf("sameSite(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
