package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const pushBatchSize = 10

func main() {
	if err := run(); err != nil {
		log.Printf("FATAL: %v", err)
		os.Exit(1)
	}
}

func run() error {
	env := loadEnv()
	httpClient := &http.Client{}

	input, err := getInput(env, httpClient)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	starts, rejected := StartUrlsFromInput(input)
	for _, r := range rejected {
		log.Printf("skipping empty entry: %q", r)
	}
	if len(starts) == 0 {
		return fmt.Errorf("no valid urls in input — provide at least one via " +
			"%q (or its alias %q)", "startUrls", "urls")
	}

	maxPages := resolveMaxPages(input.MaxPages)
	maxDepth := resolveMaxDepth(input.MaxDepth)
	followExternal := resolveFollowExternalLinks(input.FollowExternalLinks)
	timeoutSecs := resolveTimeoutSeconds(input.TimeoutSeconds)
	log.Printf("crawling %d start url(s): maxPages=%d maxDepth=%d followExternalLinks=%v timeoutSecs=%d",
		len(starts), maxPages, maxDepth, followExternal, timeoutSecs)

	pageClient := &http.Client{
		Timeout: time.Duration(timeoutSecs) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // fetchWithRedirects follows hops itself
		},
	}
	linkClient := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	robots := newRobotsCache()

	pricing := newPricingManager(env, httpClient, primaryChargeEvent)
	writer := newResultWriter[Result](env, httpClient)

	results := crawlSite(pageClient, linkClient, robots, starts, maxDepth, maxPages, followExternal, pricing)

	charged := 0
	for i := 0; i < len(results); i += pushBatchSize {
		end := i + pushBatchSize
		if end > len(results) {
			end = len(results)
		}
		batch := results[i:end]
		if err := writer.Push(batch); err != nil {
			return fmt.Errorf("pushing results to dataset: %w", err)
		}
		for _, r := range batch {
			if r.Charged {
				charged++
			}
		}
	}

	log.Printf("done: %d page(s) crawled, %d charged", len(results), charged)
	return nil
}
