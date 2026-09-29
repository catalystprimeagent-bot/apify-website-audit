package main

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// pageData is everything extracted from one parsed HTML document, before any network calls to
// check the links it found.
type pageData struct {
	title                 string
	metaDescriptionLength int
	hasStructuredData     bool
	imagesMissingAlt      int
	mixedContentFound     bool
	links                 []string // absolute, deduped, in first-seen order
}

// resourceAttrTags maps element names to the attribute that carries a fetchable resource URL,
// for the mixed-content check. Anchor hrefs are handled separately since a hyperlink is
// navigation, not an embedded resource, and is not what "mixed content" warnings are about.
var resourceAttrTags = map[string]string{
	"img":    "src",
	"script": "src",
	"iframe": "src",
	"source": "src",
	"embed":  "src",
	"audio":  "src",
	"video":  "src",
	"link":   "href", // stylesheets, preloads
}

func parsePage(body []byte, pageURL *url.URL) pageData {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return pageData{links: []string{}}
	}

	pd := pageData{links: []string{}}
	seenLinks := make(map[string]bool)
	pageIsHTTPS := pageURL.Scheme == "https"

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if pd.title == "" {
					pd.title = strings.TrimSpace(textContent(n))
				}
			case "meta":
				if attr(n, "name") == "description" {
					pd.metaDescriptionLength = len(attr(n, "content"))
				}
			case "script":
				if attr(n, "type") == "application/ld+json" && strings.TrimSpace(textContent(n)) != "" {
					pd.hasStructuredData = true
				}
			case "img":
				alt, ok := attrOK(n, "alt")
				if !ok || strings.TrimSpace(alt) == "" {
					pd.imagesMissingAlt++
				}
			case "a":
				if href := attr(n, "href"); href != "" {
					if abs := resolveLink(pageURL, href); abs != "" && !seenLinks[abs] {
						seenLinks[abs] = true
						pd.links = append(pd.links, abs)
					}
				}
			}
			if hasAttr(n, "itemscope") || hasAttr(n, "itemtype") {
				pd.hasStructuredData = true
			}
			if pageIsHTTPS && !pd.mixedContentFound {
				if attrName, ok := resourceAttrTags[n.Data]; ok {
					if isMixedContentURL(attr(n, attrName)) {
						pd.mixedContentFound = true
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return pd
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func attr(n *html.Node, key string) string {
	v, _ := attrOK(n, key)
	return v
}

func attrOK(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

func hasAttr(n *html.Node, key string) bool {
	_, ok := attrOK(n, key)
	return ok
}

// resolveLink turns a possibly-relative href into an absolute URL string, dropping the
// fragment (dedupe by page identity, not by scroll anchor) and skipping non-http(s) schemes
// (mailto:, tel:, javascript:, #fragment-only).
func resolveLink(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	u, err := base.Parse(href)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

// isMixedContentURL reports whether a resource reference on an HTTPS page is fetched over
// plain HTTP. A protocol-relative URL ("//cdn.example.com/x.js") and a relative path both
// inherit the page's own (HTTPS) scheme, so neither counts.
func isMixedContentURL(ref string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref)), "http://")
}

// sameSite reports whether two hosts belong to the same site for the purpose of the
// followExternalLinks switch, treating a "www." prefix as not a different site.
func sameSite(hostA, hostB string) bool {
	return strings.TrimPrefix(strings.ToLower(hostA), "www.") == strings.TrimPrefix(strings.ToLower(hostB), "www.")
}
