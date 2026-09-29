package main

// Input is this Actor's input schema in Go form. Keep .actor/input_schema.json in sync with
// whatever fields live here — schemaVersion 1, and leave "required" empty since startUrls/urls
// are handled as aliases in code, not by the schema (memory/015).
type Input struct {
	StartUrls           interface{} `json:"startUrls"`
	Urls                interface{} `json:"urls"`
	MaxPages            *int        `json:"maxPages"`
	MaxDepth            *int        `json:"maxDepth"`
	FollowExternalLinks *bool       `json:"followExternalLinks"`
	TimeoutSeconds      *int        `json:"timeoutSeconds"`
}

// Result is one dataset row: one crawled page. Read this struct directly when updating
// .actor/dataset_schema.json rather than guessing field names from a README example
// (memory/017: Apify's output/dataset schema publish gate never shows up in an API error).
type Result struct {
	URL                   string   `json:"url"`
	OK                    bool     `json:"ok"`
	HTTPStatus            int      `json:"httpStatus,omitempty"`
	Depth                 int      `json:"depth"`
	Title                 string   `json:"title"`
	MetaDescriptionLength int      `json:"metaDescriptionLength"`
	BrokenLinks           []string `json:"brokenLinks"`
	RedirectChain         []string `json:"redirectChain"`
	HasStructuredData     bool     `json:"hasStructuredData"`
	ImagesMissingAlt      int      `json:"imagesMissingAlt"`
	MixedContentFound     bool     `json:"mixedContentFound"`
	LatencyMs             int64    `json:"latencyMs"`
	Charged               bool     `json:"charged"`
	Error                 string   `json:"error,omitempty"`
}
