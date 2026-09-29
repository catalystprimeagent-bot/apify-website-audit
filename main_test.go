package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCrawlSite_SingleUnreachablePage_NeverCharges covers the memory/031 guard end to end: a
// start URL that cannot be reached at all must produce a row with no data and Charged=false.
func TestCrawlSite_SingleUnreachablePage_NeverCharges(t *testing.T) {
	pricing := &pricingManager{enabled: true, eventPriceUSD: 1} // even if pricing were live
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	results := crawlSite(pageClient, linkClient, robots, []string{"http://127.0.0.1:0"}, 1, 5, false, pricing)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.OK {
		t.Fatalf("expected OK=false for an unreachable address")
	}
	if r.Error == "" {
		t.Fatalf("expected a non-empty Error for an unreachable address")
	}
	if r.Charged {
		t.Fatalf("memory/031 guard violated: charged a row with no data")
	}
}

// TestCrawlSite_FollowsInternalLinksWithinDepth builds a tiny two-page site (root links to
// /page2) and confirms the crawler follows that link at depth 1 and reports both pages.
func TestCrawlSite_FollowsInternalLinksWithinDepth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Home</title></head><body><a href="/page2">next</a></body></html>`))
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Page 2</title></head><body>no links here</body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pricing := &pricingManager{enabled: false}
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	results := crawlSite(pageClient, linkClient, robots, []string{srv.URL}, 2, 10, false, pricing)
	if len(results) != 2 {
		t.Fatalf("expected 2 pages crawled (root + /page2), got %d: %+v", len(results), results)
	}
	titles := map[string]bool{}
	for _, r := range results {
		titles[r.Title] = true
	}
	if !titles["Home"] || !titles["Page 2"] {
		t.Fatalf("expected both pages' titles present, got %v", titles)
	}
}

// TestCrawlSite_RespectsMaxDepth confirms a link discovered at the deepest allowed level is not
// itself followed.
func TestCrawlSite_RespectsMaxDepth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/page2">next</a>`))
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/page3">next</a>`))
	})
	mux.HandleFunc("/page3", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`should not be fetched`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pricing := &pricingManager{enabled: false}
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	// maxDepth=1: root (depth 0) is fetched, /page2 (depth 1, discovered from root) is fetched,
	// /page3 (would be depth 2) must not be.
	results := crawlSite(pageClient, linkClient, robots, []string{srv.URL}, 1, 10, false, pricing)
	if len(results) != 2 {
		t.Fatalf("expected exactly 2 pages at maxDepth=1, got %d: %+v", len(results), results)
	}
}

// TestCrawlSite_RespectsMaxPages confirms the global page budget is enforced across a wide
// (not deep) link structure.
func TestCrawlSite_RespectsMaxPages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/a">a</a><a href="/b">b</a><a href="/c">c</a>`))
	})
	for _, p := range []string{"/a", "/b", "/c"} {
		p := p
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("leaf"))
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pricing := &pricingManager{enabled: false}
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	results := crawlSite(pageClient, linkClient, robots, []string{srv.URL}, 3, 2, false, pricing)
	if len(results) != 2 {
		t.Fatalf("expected exactly 2 pages at maxPages=2, got %d: %+v", len(results), results)
	}
}

// TestCrawlSite_ExternalLinksNotFollowedByDefault confirms an off-site link is not crawled as
// its own page unless followExternalLinks is set — it may still be checked for brokenness
// (that check is deliberately independent of the follow decision), but must not appear as a
// separate row in the results.
func TestCrawlSite_ExternalLinksNotFollowedByDefault(t *testing.T) {
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer external.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="` + external.URL + `/">off-site</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pricing := &pricingManager{enabled: false}
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	results := crawlSite(pageClient, linkClient, robots, []string{srv.URL}, 2, 10, false, pricing)
	if len(results) != 1 {
		t.Fatalf("expected only the root page crawled, got %d: %+v", len(results), results)
	}
}

// TestCrawlSite_ReportsBrokenLink confirms a dead link discovered on a page is reported in
// that page's brokenLinks.
func TestCrawlSite_ReportsBrokenLink(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<a href="/dead">dead link</a>`))
	})
	mux.HandleFunc("/dead", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pricing := &pricingManager{enabled: false}
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	// maxDepth=0 so /dead is checked as a link but never crawled as its own page.
	results := crawlSite(pageClient, linkClient, robots, []string{srv.URL}, 0, 10, false, pricing)
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 page (maxDepth=0), got %d: %+v", len(results), results)
	}
	if len(results[0].BrokenLinks) != 1 {
		t.Fatalf("expected 1 broken link reported, got %v", results[0].BrokenLinks)
	}
}

// TestCrawlSite_RedirectChainRecorded confirms a redirecting page's chain is captured and the
// final page's own content is what gets audited.
func TestCrawlSite_RedirectChainRecorded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<title>New Page</title>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pricing := &pricingManager{enabled: false}
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	results := crawlSite(pageClient, linkClient, robots, []string{srv.URL + "/old"}, 0, 5, false, pricing)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if len(r.RedirectChain) != 1 || r.RedirectChain[0] != srv.URL+"/new" {
		t.Fatalf("expected redirect chain [%s/new], got %v", srv.URL, r.RedirectChain)
	}
	if r.Title != "New Page" {
		t.Fatalf("expected the final page's own title, got %q", r.Title)
	}
	if r.HTTPStatus != http.StatusOK {
		t.Fatalf("expected final status 200, got %d", r.HTTPStatus)
	}
}

// TestCrawlSite_ArrayFieldsNeverSerializeAsNull guards against a real bug caught on a live
// Apify run: checkBrokenLinks returned a nil slice when nothing was broken, which json.Marshal
// encodes as `null` rather than `[]`. Apify's dataset schema types brokenLinks/redirectChain as
// arrays and rejects null with a 400 schema-validation-error on push — a page with a clean bill
// of health could never be saved. Assert on the actual marshaled JSON, not just len() == 0,
// since a nil slice and an empty slice are equal under len() but differ under json.Marshal.
func TestCrawlSite_ArrayFieldsNeverSerializeAsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>No links here</title></head><body>plain text</body></html>`))
	}))
	defer srv.Close()

	pricing := &pricingManager{enabled: false}
	pageClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	linkClient := &http.Client{}
	robots := newRobotsCache()

	results := crawlSite(pageClient, linkClient, robots, []string{srv.URL}, 0, 5, false, pricing)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	b, err := json.Marshal(results[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"brokenLinks", "redirectChain"} {
		if raw[field] == nil {
			t.Fatalf("%s serialized as JSON null (got %s) -- Apify's dataset schema rejects this", field, b)
		}
	}
}
