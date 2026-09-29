package main

import "testing"

func TestParseRobots_DisallowUnderWildcard(t *testing.T) {
	body := "User-agent: *\nDisallow: /private/\n"
	rules := parseRobots(body)
	if isPathAllowed(rules, "/private/secret") {
		t.Errorf("expected /private/secret to be disallowed")
	}
	if !isPathAllowed(rules, "/public") {
		t.Errorf("expected /public to be allowed")
	}
}

func TestParseRobots_OwnUserAgentGroupTakesPrecedence(t *testing.T) {
	body := "User-agent: *\nDisallow: /\n" +
		"User-agent: catalystwebsiteauditbot\nDisallow: /admin/\n"
	rules := parseRobots(body)
	if !isPathAllowed(rules, "/anything") {
		t.Errorf("our specific group allows everything except /admin/, wildcard group should not apply")
	}
	if isPathAllowed(rules, "/admin/secret") {
		t.Errorf("expected /admin/secret to be disallowed")
	}
}

func TestParseRobots_AllowWinsOnLongerMatch(t *testing.T) {
	body := "User-agent: *\nDisallow: /docs\nAllow: /docs/public\n"
	rules := parseRobots(body)
	if isPathAllowed(rules, "/docs/private") {
		t.Errorf("expected /docs/private to be disallowed")
	}
	if !isPathAllowed(rules, "/docs/public/page") {
		t.Errorf("expected the more specific Allow to win for /docs/public/page")
	}
}

func TestParseRobots_EmptyBodyAllowsEverything(t *testing.T) {
	rules := parseRobots("")
	if !isPathAllowed(rules, "/anything") {
		t.Errorf("expected an empty robots.txt to allow everything")
	}
}

func TestParseRobots_EmptyDisallowMeansAllowAll(t *testing.T) {
	rules := parseRobots("User-agent: *\nDisallow:\n")
	if !isPathAllowed(rules, "/anything") {
		t.Errorf("a bare 'Disallow:' means allow everything, not block everything")
	}
}
