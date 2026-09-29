#!/usr/bin/env python3
"""Create, run, verify and publish this Actor's task examples.

Generated for website-audit by actors/scaffold/generate.py. The recipe (memory/030): a task example
is created off /v2/actor-tasks (not the Actor directly), run once, and its rows are read and
checked with the fail-closed gate in verify_gate.py *before* it is ever made public. Publishing
sets isPublic plus a publicConfig block that does not exist on the object returned by a plain
GET/POST -- see memory/030 for why guessing its shape from the error message doesn't work.

Fill in ACT_ID and EXAMPLES below, then:

    python task_examples.py create              # create/update + run + verify every example
    python task_examples.py publish <name> ...   # publish only the names that passed

Never publish a name that printed HOLD. That is the whole point of running this in two stages.
"""
import json
import os
import sys
import time
import urllib.error
import urllib.request

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "scaffold"))
from verify_gate import GateConfig, verify_dataset  # noqa: E402

TOKEN = os.environ["APIFY_TOKEN"]
API = "https://api.apify.com/v2"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "task-example-runs.md")

ACT_ID = "WVEdB5OryAODEU3Eh"

# Real, ToS-safe public sites only, each already run once for real against the live Actor
# (see runtime/handoffs) before being written here, per memory/031: run the showcase before you
# publish it, don't guess the copy from what the code is supposed to do.
EXAMPLES = [
    dict(
        name="audit-a-single-page",
        title="Audit a single page's technical SEO",
        description="Checks one URL's title, meta description, structured data, alt text "
                     "coverage and mixed content without following any links.",
        input={"startUrls": ["https://example.com"], "maxDepth": 0, "maxPages": 1},
        seoTitle="Audit a single page's SEO health",
        seoDescription="Check one page's title, meta description, structured data and alt "
                        "text coverage in one request, no crawling.",
        fields=["startUrls", "maxDepth"],
    ),
    dict(
        name="find-broken-links-across-a-site",
        title="Find broken links across a site",
        description="Crawls one level deep from a start URL and reports every broken link "
                     "and redirect chain found on each page, internal or external.",
        input={"startUrls": ["https://www.python.org"], "maxDepth": 1, "maxPages": 10},
        seoTitle="Find broken links on a website",
        seoDescription="Crawl a site one level deep and get every page's broken links and "
                        "redirect chains in a single dataset.",
        fields=["startUrls", "maxDepth", "maxPages"],
    ),
    dict(
        name="check-accessibility-and-structured-data",
        title="Check alt text coverage and structured data",
        description="Audits a single page for images missing alt text and the presence of "
                     "JSON-LD or microdata structured data.",
        input={"startUrls": ["https://vercel.com"], "maxDepth": 0, "maxPages": 1},
        seoTitle="Check page alt text and structured data",
        seoDescription="Find images missing alt text and confirm whether a page has "
                        "structured data, in one request.",
        fields=["startUrls", "maxDepth"],
    ),
    dict(
        name="crawl-a-site-section",
        title="Crawl a section of a site up to a page budget",
        description="Crawls up to a fixed page budget starting from a section URL, following "
                     "internal links two levels deep, and reports every page's audit fields.",
        input={"startUrls": ["https://www.python.org/about/"], "maxDepth": 2, "maxPages": 15},
        seoTitle="Crawl a website section for an SEO audit",
        seoDescription="Crawl up to a page budget from a start URL, two levels of internal "
                        "links deep, and get a technical SEO audit of every page found.",
        fields=["startUrls", "maxDepth", "maxPages"],
    ),
]

# httpStatus is the one field guaranteed non-empty on every successfully fetched page
# (real audit data even for a 404), so it is the billing-defect data field per memory/031: a
# charged row with no httpStatus would be the defect this gate exists to catch.
GATE_CONFIG = GateConfig(
    error_field="error",
    required_present_fields=["depth", "brokenLinks", "redirectChain", "hasStructuredData",
                              "imagesMissingAlt", "mixedContentFound"],
    success_field="ok",
    success_value=True,
    charge_field="charged",
    data_fields=["httpStatus"],
)

RUN_OPTIONS = None  # e.g. {"build": "latest", "memoryMbytes": 512, "timeoutSecs": 120}


def call(method, path, body=None, query=""):
    url = f"{API}{path}{query}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {TOKEN}")
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            return json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        raise SystemExit(f"HTTP {e.code} on {method} {path}\n{e.read().decode()[:1200]}")


def check_copy(e):
    """Limits are enforced server side. Count before spending a call."""
    bad = []
    if not (3 <= len(e["title"]) <= 63):
        bad.append(f"title {len(e['title'])} chars")
    if len(e["description"]) > 400:
        bad.append(f"description {len(e['description'])} chars")
    if len(e["seoTitle"]) > 60:
        bad.append(f"seoTitle {len(e['seoTitle'])} chars")
    if len(e["seoDescription"]) > 160:
        bad.append(f"seoDescription {len(e['seoDescription'])} chars")
    for k in ("title", "description", "seoTitle", "seoDescription"):
        if "—" in e[k] or " - " in e[k]:
            bad.append(f"{k} has an em-dash or ' - ' separator")
    for f in e["fields"]:
        if f not in e["input"]:
            bad.append(f"inputSchemaFields names {f}, absent from input")
    return bad


def do_create():
    if ACT_ID == "__FILL_IN_ACTOR_ID__":
        raise SystemExit("fill in ACT_ID at the top of this file first")
    if not EXAMPLES:
        raise SystemExit("EXAMPLES is empty -- add at least one dict before running create")

    problems = {e["name"]: check_copy(e) for e in EXAMPLES}
    if any(problems.values()):
        for n, p in problems.items():
            if p:
                print(f"COPY FAIL {n}: {p}")
        raise SystemExit("fix copy before spending API calls")
    print(f"copy limits ok for all {len(EXAMPLES)}\n")

    existing = {t["name"]: t["id"] for t in call("GET", "/actor-tasks", query="?limit=100")["data"]["items"]}
    report = []

    for e in EXAMPLES:
        print(f"===== {e['name']} =====")
        body = {"actId": ACT_ID, "name": e["name"], "title": e["title"],
                "description": e["description"], "input": e["input"]}
        if RUN_OPTIONS:
            body["options"] = RUN_OPTIONS
        if e["name"] in existing:
            tid = existing[e["name"]]
            call("PUT", f"/actor-tasks/{tid}", {k: v for k, v in body.items() if k != "actId"})
            print(f"  exists, input resynced: {tid}")
        else:
            tid = call("POST", "/actor-tasks", body)["data"]["id"]
            print(f"  created: {tid}")

        run = call("POST", f"/actor-tasks/{tid}/runs", query="?waitForFinish=150")["data"]
        rid, status = run["id"], run["status"]
        for _ in range(30):
            if status not in ("RUNNING", "READY"):
                break
            time.sleep(5)
            run = call("GET", f"/actor-runs/{rid}")["data"]
            status = run["status"]
        dsid = run["defaultDatasetId"]
        items = call("GET", f"/datasets/{dsid}/items", query="?clean=true&limit=100")
        print(f"  run {rid} {status}, {len(items)} rows")

        ok, gate_problems = verify_dataset(items, GATE_CONFIG)
        for p in gate_problems:
            print(f"    FAIL {p}")
        verdict = "PASS" if ok else f"HOLD ({len(gate_problems)} problem(s))"
        print(f"  VERDICT: {verdict}\n")
        report.append(dict(name=e["name"], tid=tid, rid=rid, status=status,
                            rows=len(items), verdict=verdict, problems=gate_problems))

    with open(OUT, "w", encoding="utf-8") as f:
        f.write(f"# website-audit task example pre-publish runs\n\n")
        f.write("Every row read through verify_gate.py before any publish, per memory/031 and "
                "memory/033. Generated by task_examples.py create.\n\n")
        for r in report:
            f.write(f"## {r['name']}\n\n")
            f.write(f"- task `{r['tid']}`, run `{r['rid']}` {r['status']}, {r['rows']} rows\n")
            f.write(f"- **{r['verdict']}**\n")
            for p in r["problems"]:
                f.write(f"  - {p}\n")
            f.write("\n")
    print(f"wrote {OUT}")
    print("PASS:", [r["name"] for r in report if r["verdict"] == "PASS"])
    print("HOLD:", [r["name"] for r in report if r["verdict"] != "PASS"])


def do_publish(names):
    existing = {t["name"]: t["id"] for t in call("GET", "/actor-tasks", query="?limit=100")["data"]["items"]}
    by_name = {e["name"]: e for e in EXAMPLES}
    for name in names:
        if name not in by_name:
            print(f"skip {name}: not in EXAMPLES")
            continue
        e = by_name[name]
        tid = existing[name]
        res = call("PUT", f"/actor-tasks/{tid}", {
            "isPublic": True,
            "publicConfig": {
                "inputSchemaFields": e["fields"],
                "datasetView": "overview",
                "seoTitle": e["seoTitle"],
                "seoDescription": e["seoDescription"],
            },
        })["data"]
        pc = res.get("publicConfig") or {}
        print(f"{name}: isPublic={res.get('isPublic')} publishedAt={pc.get('publishedAt')}")


if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] not in ("create", "publish"):
        raise SystemExit("usage: task_examples.py create | publish <name> [name...]")
    if sys.argv[1] == "create":
        do_create()
    else:
        do_publish(sys.argv[2:])
