package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	userAgent = "CatalystWebsiteAuditBot/1.0 (+https://apify.com/catalyst_prime/website-audit)"

	maxRedirectsFollowed  = 10
	maxLinksCheckedPerPage = 20
	linkCheckConcurrency  = 8
	pageFetchConcurrency  = 8
	maxBodyBytes          = 5 << 20 // 5 MB per page, keeps memory bounded on large pages
)

// frontierItem is one page still to be crawled. originHost is the host of the start URL that
// this branch of the crawl began from — followExternalLinks compares against it, not against
// the immediate parent page, so a same-site link found via an off-site page (once external
// following is on) doesn't wrongly re-anchor the site.
type frontierItem struct {
	url        string
	depth      int
	originHost string
}

// pageBudget is the global cap on how many pages this run will actually fetch, shared across
// every start URL and every depth. Claimed atomically so concurrent workers never overshoot it.
type pageBudget struct {
	mu        sync.Mutex
	remaining int
}

func newPageBudget(n int) *pageBudget { return &pageBudget{remaining: n} }

func (b *pageBudget) claim() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

// healthCache remembers whether a URL is broken (network error or HTTP >= 400), keyed by
// normalizeKey. Populated both by pages we actually crawl and by link-only HEAD/GET checks, and
// read by both, so the same URL is never fetched twice over the course of one run.
type healthCache struct {
	mu     sync.Mutex
	broken map[string]bool
}

func newHealthCache() *healthCache { return &healthCache{broken: make(map[string]bool)} }

func (h *healthCache) get(key string) (bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.broken[key]
	return v, ok
}

func (h *healthCache) set(key string, isBroken bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broken[key] = isBroken
}

// crawlSite runs a breadth-first crawl bounded by maxPages (total, across every start URL) and
// maxDepth (link-hops from whichever start URL began that branch). It returns one Result per
// page actually fetched, in no particular cross-level order (each level's own fetches run
// concurrently).
func crawlSite(pageClient, linkClient *http.Client, robots *robotsCache, starts []string, maxDepth, maxPages int, followExternal bool, pricing *pricingManager) []Result {
	visited := make(map[string]bool)
	var visitedMu sync.Mutex
	markVisited := func(key string) bool {
		visitedMu.Lock()
		defer visitedMu.Unlock()
		if visited[key] {
			return false
		}
		visited[key] = true
		return true
	}

	budget := newPageBudget(maxPages)
	health := newHealthCache()

	var allResults []Result
	var frontier []frontierItem

	for _, raw := range starts {
		u := ensureScheme(raw)
		parsed, err := url.Parse(u)
		if err != nil || parsed.Host == "" {
			allResults = append(allResults, Result{
				URL: raw, Error: "invalid URL", BrokenLinks: []string{}, RedirectChain: []string{},
			})
			continue
		}
		if !markVisited(normalizeKey(u)) {
			continue // duplicate start URL
		}
		frontier = append(frontier, frontierItem{url: u, depth: 0, originHost: parsed.Host})
	}

	for depth := 0; len(frontier) > 0; depth++ {
		results, discovered := processLevel(pageClient, linkClient, robots, frontier, budget, health, pricing)
		allResults = append(allResults, results...)

		if depth >= maxDepth {
			break
		}

		var next []frontierItem
		for _, kid := range discovered {
			if !followExternal && !sameSite(kid.originHost, hostOf(kid.url)) {
				continue
			}
			if !markVisited(normalizeKey(kid.url)) {
				continue
			}
			next = append(next, frontierItem{url: kid.url, depth: depth + 1, originHost: kid.originHost})
		}
		frontier = next
	}

	return allResults
}

// processLevel fetches every item in one BFS level concurrently and returns both the dataset
// rows produced and the raw (not yet deduped or depth/follow-filtered) links they discovered.
func processLevel(pageClient, linkClient *http.Client, robots *robotsCache, items []frontierItem, budget *pageBudget, health *healthCache, pricing *pricingManager) ([]Result, []frontierItem) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []Result
	var children []frontierItem

	sem := make(chan struct{}, pageFetchConcurrency)
	for _, item := range items {
		item := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			parsed, err := url.Parse(item.url)
			if err != nil {
				mu.Lock()
				results = append(results, Result{
					URL: item.url, Depth: item.depth, Error: "invalid URL: " + err.Error(),
					BrokenLinks: []string{}, RedirectChain: []string{},
				})
				mu.Unlock()
				return
			}
			if !robots.Allowed(parsed) {
				return // robots.txt disallows us: not crawled, not reported, budget untouched
			}
			if !budget.claim() {
				return // maxPages reached
			}

			res, kids := fetchAndAudit(pageClient, linkClient, item, health, pricing)
			mu.Lock()
			results = append(results, res)
			children = append(children, kids...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results, children
}

// fetchAndAudit fetches one page, runs every technical-SEO check against it, and returns the
// dataset row plus the outbound links it found (for the caller to filter and enqueue).
func fetchAndAudit(pageClient, linkClient *http.Client, item frontierItem, health *healthCache, pricing *pricingManager) (Result, []frontierItem) {
	result := Result{URL: item.url, Depth: item.depth, BrokenLinks: []string{}, RedirectChain: []string{}}

	start := time.Now()
	resp, finalURL, chain, err := fetchWithRedirects(pageClient, item.url)
	result.LatencyMs = time.Since(start).Milliseconds()
	if chain != nil {
		result.RedirectChain = chain
	}

	if err != nil {
		result.Error = err.Error()
		health.set(normalizeKey(item.url), true)
		return result, nil // no response at all: never charged
	}
	defer resp.Body.Close()

	result.OK = true
	result.HTTPStatus = resp.StatusCode
	broken := resp.StatusCode >= 400
	health.set(normalizeKey(item.url), broken)
	health.set(normalizeKey(finalURL), broken)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))

	finalParsed, perr := url.Parse(finalURL)
	if perr != nil {
		finalParsed, _ = url.Parse(item.url)
	}

	var kids []frontierItem
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		pd := parsePage(body, finalParsed)
		result.Title = pd.title
		result.MetaDescriptionLength = pd.metaDescriptionLength
		result.HasStructuredData = pd.hasStructuredData
		result.ImagesMissingAlt = pd.imagesMissingAlt
		result.MixedContentFound = pd.mixedContentFound
		result.BrokenLinks = checkBrokenLinks(linkClient, pd.links, health)
		for _, l := range pd.links {
			kids = append(kids, frontierItem{url: l, originHost: item.originHost})
		}
	}

	// httpStatus is real, returnable data even for a 404/500 response (the audit's whole job is
	// to surface exactly that), so any completed request is charged (memory/031's guard is about
	// requests that returned nothing at all, handled by the err != nil branch above).
	result.Charged = pricing.Charge(item.url)
	return result, kids
}

// fetchWithRedirects follows redirects itself (rather than letting *http.Client do it) so it
// can report the exact hop sequence. pageClient must have CheckRedirect set to always return
// http.ErrUseLastResponse. chain holds every URL after the first, in order, ending with
// finalURL; it is empty when there was no redirect.
func fetchWithRedirects(pageClient *http.Client, startURL string) (resp *http.Response, finalURL string, chain []string, err error) {
	currentURL := startURL
	chain = []string{}

	for hop := 0; hop < maxRedirectsFollowed; hop++ {
		req, err := http.NewRequest(http.MethodGet, currentURL, nil)
		if err != nil {
			return nil, currentURL, chain, err
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err = pageClient.Do(req)
		if err != nil {
			return nil, currentURL, chain, err
		}

		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			return resp, currentURL, chain, nil
		}

		loc := resp.Header.Get("Location")
		resp.Body.Close()
		if loc == "" {
			return resp, currentURL, chain, nil // redirect status with no Location: treat as final
		}
		base, _ := url.Parse(currentURL)
		next, perr := base.Parse(loc)
		if perr != nil {
			return resp, currentURL, chain, nil
		}
		currentURL = next.String()
		chain = append(chain, currentURL)
	}
	return nil, currentURL, chain, fmt.Errorf("too many redirects (>%d)", maxRedirectsFollowed)
}

// checkBrokenLinks returns the subset of links (in their original order) found to be broken.
// Links already known from a prior fetch or check anywhere in this run's healthCache are reused
// rather than re-requested; only genuinely new links are checked, and only up to
// maxLinksCheckedPerPage of them, to keep one page's audit cost bounded.
func checkBrokenLinks(linkClient *http.Client, links []string, health *healthCache) []string {
	var toCheck []string
	seen := make(map[string]bool)
	for _, l := range links {
		key := normalizeKey(l)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, known := health.get(key); !known {
			toCheck = append(toCheck, l)
		}
	}
	if len(toCheck) > maxLinksCheckedPerPage {
		toCheck = toCheck[:maxLinksCheckedPerPage]
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, linkCheckConcurrency)
	for _, l := range toCheck {
		l := l
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			health.set(normalizeKey(l), isLinkBroken(linkClient, l))
		}()
	}
	wg.Wait()

	broken := []string{} // never nil: json.Marshal would emit null, which fails the dataset schema's array type
	seenBroken := make(map[string]bool)
	for _, l := range links {
		key := normalizeKey(l)
		if seenBroken[key] {
			continue
		}
		if v, ok := health.get(key); ok && v {
			seenBroken[key] = true
			broken = append(broken, l)
		}
	}
	return broken
}

// isLinkBroken reports whether a single link is broken: a network-level failure, or an HTTP
// status >= 400. Tries HEAD first (cheaper) and falls back to GET when a server doesn't support
// HEAD (405/501) or refuses the request outright.
func isLinkBroken(client *http.Client, link string) bool {
	if status, err := requestStatus(client, http.MethodHead, link); err == nil {
		if status != http.StatusMethodNotAllowed && status != http.StatusNotImplemented {
			return status >= 400
		}
	}
	status, err := requestStatus(client, http.MethodGet, link)
	if err != nil {
		return true
	}
	return status >= 400
}

func requestStatus(client *http.Client, method, link string) (int, error) {
	req, err := http.NewRequest(method, link, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return resp.StatusCode, nil
}

// ensureScheme prepends https:// to a bare host/path input ("example.com/pricing"), matching
// how a buyer is likely to type a start URL.
func ensureScheme(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return "https://" + raw
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// normalizeKey canonicalizes a URL for dedupe/cache-key purposes: lowercase scheme and host, a
// single trailing slash trimmed from any non-root path, fragment dropped, query re-encoded in a
// stable order.
func normalizeKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	path := u.Path
	if path == "" {
		path = "/"
	} else if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	key := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + path
	if q := u.Query().Encode(); q != "" {
		key += "?" + q
	}
	return key
}
