# Website Technical & SEO Audit Crawler

**Run it on Apify:
[apify.com/catalyst_prime/website-audit](https://apify.com/catalyst_prime/website-audit)** — free,
no setup, runs in the browser.

Crawls a list of start URLs and reports the technical-SEO health of every page found: broken
links, redirect chains, missing meta descriptions, alt-text coverage, structured data and mixed
content.

This repository is the Actor's source. The Actor itself runs on the
[Apify platform](https://apify.com/catalyst_prime/website-audit).

**Not a content extractor.** This does not convert pages to Markdown for an LLM/RAG pipeline —
that job is already well served on the Store. This tool answers a different question: *is this
page's plumbing broken?*

## Input

| Field | Type | Default | Description |
|---|---|---|---|
| `startUrls` | array of strings | none | Start URLs to crawl. Supply this or the alias below. Accepts a JSON array, a single string, or a comma/newline separated list. |
| `urls` | array | none | Alias of `startUrls`. |
| `maxPages` | integer | `20` | Total pages to crawl across every start URL combined. Clamped to 1-50. |
| `maxDepth` | integer | `2` | How many link-hops from each start URL to follow. `0` audits only the start URLs themselves. Clamped to 0-5. |
| `followExternalLinks` | boolean | `false` | When `true`, links to a different domain than the start URL are crawled too (still counted against `maxPages`/`maxDepth`). When `false`, off-site links are still checked and reported in `brokenLinks`, just not crawled as their own pages. |
| `timeoutSeconds` | integer | `15` | Per-request timeout, clamped to 3-60. |

## Output

One dataset row per page actually crawled:

```json
{
  "url": "https://example.com/",
  "ok": true,
  "httpStatus": 200,
  "depth": 0,
  "title": "Example Domain",
  "metaDescriptionLength": 0,
  "brokenLinks": ["https://example.com/old-page"],
  "redirectChain": [],
  "hasStructuredData": false,
  "imagesMissingAlt": 2,
  "mixedContentFound": false,
  "latencyMs": 143,
  "charged": false
}
```

- `url` is the page as requested, before any redirects; `httpStatus` and the parsed fields
  (`title`, `metaDescriptionLength`, etc.) reflect the **final** page after following them.
- `redirectChain` lists every URL hopped through after the first request, in order, ending with
  the final URL. Empty when there was no redirect.
- `brokenLinks` lists links found on the page (internal or external) that returned a network
  error or an HTTP status of 400+, checked up to 20 per page. A link already known from having
  been crawled or checked elsewhere in the same run is reused rather than re-requested.
- `hasStructuredData` is `true` if the page has a non-empty `application/ld+json` script or a
  microdata (`itemscope`) element.
- `imagesMissingAlt` counts `<img>` tags with no `alt` attribute or an empty one.
- `mixedContentFound` is `true` only for an HTTPS page that loads a resource (script, image,
  stylesheet, iframe, etc.) over plain `http://`. A protocol-relative URL (`//cdn...`) is not
  mixed content.
- `httpStatus` and the parsed fields are omitted from a row whose request never completed at
  all; `error` is set on that row instead. `charged` reports whether the row was billed (see
  Pricing) — a row is charged whenever a response was actually received, even a 404 or 500,
  since the status itself is the data this Actor sells.

## How the crawl works

Breadth-first from each start URL. `maxPages` is a single global budget shared across every
start URL and every depth — once it's spent, no further pages are fetched, wherever they were
discovered. `maxDepth` counts link-hops from whichever start URL began that branch, not from
the page that happens to link to it.

The crawler identifies itself as `CatalystWebsiteAuditBot` and reads `robots.txt` once per host
(cached for the rest of the run): a page disallowed there is skipped entirely — not fetched, not
reported, and its slot in `maxPages` is not spent. It never sends credentials, so it never
reaches anything behind a login; a page that requires one will show up as its own HTTP status
(401/403) rather than being silently skipped.

## Pricing

**Free.** This Actor has no price set, so a run costs you only your own Apify platform usage.

The code supports pay-per-event billing on a single `page-audit` event, priced per page
successfully fetched, if a price is ever set on the Apify Console. Nothing is hardcoded: it
reads its own current price from the platform at startup and runs unmetered when there isn't
one.

## Example

Crawling `https://example.com` with `maxDepth: 0` (audit just the one page, no following)
returns a single row confirming the page loads clean: `httpStatus: 200`, no broken links, no
mixed content, zero images missing alt text. Raise `maxDepth` and `maxPages` to audit a whole
section of a site in one run instead of one page at a time.

Four ready-to-run examples, each with real input and real output:

- [Audit a single page's technical SEO](https://apify.com/catalyst_prime/website-audit/examples/audit-a-single-page)
- [Find broken links across a site](https://apify.com/catalyst_prime/website-audit/examples/find-broken-links-across-a-site)
- [Check alt text coverage and structured data](https://apify.com/catalyst_prime/website-audit/examples/check-accessibility-and-structured-data)
- [Crawl a section of a site up to a page budget](https://apify.com/catalyst_prime/website-audit/examples/crawl-a-site-section)

## Other Actors from the same author

- [Email Verifier](https://apify.com/catalyst_prime/email-verifier) — syntax, MX and
  disposable-domain checks, no target site to break.
  ([source](https://github.com/catalystprimeagent-bot/apify-email-verifier))
- [Tech Stack Lookup](https://apify.com/catalyst_prime/tech-stack-lookup) — what a site runs on,
  plus TLS expiry, CDN and mail provider.
  ([source](https://github.com/catalystprimeagent-bot/apify-tech-stack-lookup))
- [Google Trends](https://apify.com/catalyst_prime/google-trends) — interest over time and related
  queries for any term. ([source](https://github.com/catalystprimeagent-bot/apify-google-trends))
