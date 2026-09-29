package main

import "testing"

func TestStartUrlsFromInput_PrimaryField(t *testing.T) {
	in := Input{StartUrls: []interface{}{"a.com", "b.com", "a.com"}}
	items, rejected := StartUrlsFromInput(in)
	if len(items) != 2 {
		t.Fatalf("expected 2 deduped items, got %v", items)
	}
	if len(rejected) != 0 {
		t.Fatalf("expected no rejected entries, got %v", rejected)
	}
}

func TestStartUrlsFromInput_AliasField(t *testing.T) {
	in := Input{Urls: "c.com"}
	items, _ := StartUrlsFromInput(in)
	if len(items) != 1 || items[0] != "c.com" {
		t.Fatalf("expected alias field to be read, got %v", items)
	}
}

func TestStartUrlsFromInput_EmptyEntryRejected(t *testing.T) {
	in := Input{StartUrls: []interface{}{"", "  ", "d.com"}}
	items, rejected := StartUrlsFromInput(in)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %v", items)
	}
	if len(rejected) == 0 {
		t.Fatalf("expected empty entries to be rejected, not silently dropped")
	}
}

func TestResolveTimeoutSeconds(t *testing.T) {
	if got := resolveTimeoutSeconds(nil); got != 15 {
		t.Errorf("default: got %d, want 15", got)
	}
	tooLow, tooHigh := 1, 999
	if got := resolveTimeoutSeconds(&tooLow); got != 3 {
		t.Errorf("clamp low: got %d, want 3", got)
	}
	if got := resolveTimeoutSeconds(&tooHigh); got != 60 {
		t.Errorf("clamp high: got %d, want 60", got)
	}
}

func TestResolveMaxPages(t *testing.T) {
	if got := resolveMaxPages(nil); got != 20 {
		t.Errorf("default: got %d, want 20", got)
	}
	tooLow, tooHigh := 0, 999
	if got := resolveMaxPages(&tooLow); got != 1 {
		t.Errorf("clamp low: got %d, want 1", got)
	}
	if got := resolveMaxPages(&tooHigh); got != 50 {
		t.Errorf("clamp high: got %d, want 50", got)
	}
}

func TestResolveMaxDepth(t *testing.T) {
	if got := resolveMaxDepth(nil); got != 2 {
		t.Errorf("default: got %d, want 2", got)
	}
	tooLow, tooHigh := -1, 999
	if got := resolveMaxDepth(&tooLow); got != 0 {
		t.Errorf("clamp low: got %d, want 0", got)
	}
	if got := resolveMaxDepth(&tooHigh); got != 5 {
		t.Errorf("clamp high: got %d, want 5", got)
	}
}

func TestResolveFollowExternalLinks(t *testing.T) {
	if got := resolveFollowExternalLinks(nil); got != false {
		t.Errorf("default: got %v, want false", got)
	}
	yes := true
	if got := resolveFollowExternalLinks(&yes); got != true {
		t.Errorf("explicit true: got %v, want true", got)
	}
}
