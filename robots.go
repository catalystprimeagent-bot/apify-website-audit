package main

import (
	"bufio"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// robotsRule is one Disallow/Allow line from a matching user-agent group.
type robotsRule struct {
	allow  bool
	prefix string
}

// robotsCache fetches and caches robots.txt per host so a multi-page crawl of the same site
// only pays for it once. A fetch failure or missing file is treated as allow-all, matching the
// de-facto standard's own fallback.
type robotsCache struct {
	mu     sync.Mutex
	rules  map[string][]robotsRule
	client *http.Client
}

func newRobotsCache() *robotsCache {
	return &robotsCache{
		rules:  make(map[string][]robotsRule),
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

// Allowed reports whether ourUserAgent may fetch u under the robots.txt of u's host.
func (c *robotsCache) Allowed(u *url.URL) bool {
	origin := u.Scheme + "://" + u.Host
	c.mu.Lock()
	rules, known := c.rules[origin]
	c.mu.Unlock()
	if !known {
		rules = c.fetch(origin)
		c.mu.Lock()
		c.rules[origin] = rules
		c.mu.Unlock()
	}
	return isPathAllowed(rules, u.EscapedPath())
}

func (c *robotsCache) fetch(origin string) []robotsRule {
	req, err := http.NewRequest(http.MethodGet, origin+"/robots.txt", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil // unreachable robots.txt: allow-all
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil
	}
	return parseRobots(string(body))
}

// parseRobots extracts the rule group that applies to us: an exact match on our own
// user-agent token if one exists, otherwise the "*" group. Only Allow/Disallow are honored;
// Crawl-delay and Sitemap lines are ignored on purpose (memory/047-style: no rail needs this
// Actor to be polite about pacing, only about scope).
func parseRobots(body string) []robotsRule {
	const ourToken = "catalystwebsiteauditbot"

	type group struct {
		agents []string
		rules  []robotsRule
	}
	var groups []group
	cur := group{}
	flushed := true

	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])

		switch key {
		case "user-agent":
			if !flushed || len(cur.rules) > 0 {
				groups = append(groups, cur)
				cur = group{}
			}
			cur.agents = append(cur.agents, strings.ToLower(val))
			flushed = false
		case "allow", "disallow":
			if val == "" && key == "disallow" {
				continue // "Disallow:" with empty value means allow everything
			}
			cur.rules = append(cur.rules, robotsRule{allow: key == "allow", prefix: val})
			flushed = false
		}
	}
	if len(cur.agents) > 0 || len(cur.rules) > 0 {
		groups = append(groups, cur)
	}

	var star, ours []robotsRule
	for _, g := range groups {
		for _, a := range g.agents {
			if a == ourToken {
				ours = append(ours, g.rules...)
			}
			if a == "*" {
				star = append(star, g.rules...)
			}
		}
	}
	if len(ours) > 0 {
		return ours
	}
	return star
}

// isPathAllowed applies the longest-prefix-match rule (ties broken in favor of Allow), the
// de-facto algorithm documented at https://developers.google.com/search/docs/crawling-indexing/robots/robots_txt.
func isPathAllowed(rules []robotsRule, path string) bool {
	if path == "" {
		path = "/"
	}
	bestLen := -1
	bestAllow := true
	for _, r := range rules {
		if !strings.HasPrefix(path, r.prefix) {
			continue
		}
		l := len(r.prefix)
		if l > bestLen || (l == bestLen && r.allow) {
			bestLen = l
			bestAllow = r.allow
		}
	}
	return bestAllow
}
