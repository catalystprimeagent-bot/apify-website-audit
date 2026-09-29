package main

import "testing"

func TestNormalizeKey(t *testing.T) {
	cases := []struct{ a, b string }{
		{"https://Example.com/Path/", "https://example.com/Path"},
		{"https://example.com", "https://example.com/"},
		{"https://example.com/x?b=2&a=1", "https://example.com/x?a=1&b=2"},
	}
	for _, c := range cases {
		if normalizeKey(c.a) != normalizeKey(c.b) {
			t.Errorf("normalizeKey(%q)=%q != normalizeKey(%q)=%q", c.a, normalizeKey(c.a), c.b, normalizeKey(c.b))
		}
	}
	if normalizeKey("https://example.com/a") == normalizeKey("https://example.com/b") {
		t.Errorf("distinct paths must not normalize to the same key")
	}
}

func TestEnsureScheme(t *testing.T) {
	cases := map[string]string{
		"example.com":         "https://example.com",
		"http://example.com":  "http://example.com",
		"https://example.com": "https://example.com",
	}
	for in, want := range cases {
		if got := ensureScheme(in); got != want {
			t.Errorf("ensureScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostOf(t *testing.T) {
	if got := hostOf("https://example.com/path"); got != "example.com" {
		t.Errorf("hostOf = %q, want example.com", got)
	}
}

func TestPageBudget_ClaimStopsAtZero(t *testing.T) {
	b := newPageBudget(2)
	if !b.claim() || !b.claim() {
		t.Fatalf("expected first two claims to succeed")
	}
	if b.claim() {
		t.Fatalf("expected third claim to fail once budget is exhausted")
	}
}

func TestHealthCache_SetAndGet(t *testing.T) {
	h := newHealthCache()
	if _, ok := h.get("k"); ok {
		t.Fatalf("expected unknown key to report not-known")
	}
	h.set("k", true)
	v, ok := h.get("k")
	if !ok || !v {
		t.Fatalf("expected k to be known and broken")
	}
}
