package main

import "strings"

// StartUrlsFromInput gathers every item the buyer supplied across the primary field and
// its alias, normalizes (trim + dedupe, case-insensitive) while preserving first-seen order.
//
// TEMPLATE: add more aliases the same way tech-stack-lookup and google-trends do (see their own
// input.go for the pattern) — always through coerceToList so arrays, {"url": "..."}-shaped
// objects, a single string, and comma/newline-separated strings all keep working the same way.
func StartUrlsFromInput(in Input) (items []string, rejected []string) {
	seen := make(map[string]bool)

	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			rejected = append(rejected, raw)
			return
		}
		key := strings.ToLower(raw)
		if seen[key] {
			return
		}
		seen[key] = true
		items = append(items, raw)
	}

	for _, raw := range coerceToList(in.StartUrls, "url") {
		add(raw)
	}
	for _, raw := range coerceToList(in.Urls, "url") {
		add(raw)
	}

	return items, rejected
}

func resolveTimeoutSeconds(in *int) int {
	const def, min, max = 15, 3, 60
	if in == nil {
		return def
	}
	v := *in
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func resolveMaxPages(in *int) int {
	const def, min, max = 20, 1, 50
	if in == nil {
		return def
	}
	v := *in
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func resolveMaxDepth(in *int) int {
	const def, min, max = 2, 0, 5
	if in == nil {
		return def
	}
	v := *in
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func resolveFollowExternalLinks(in *bool) bool {
	if in == nil {
		return false
	}
	return *in
}
