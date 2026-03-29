# Modularise URL Retrieval with Cloudflare Browser Rendering

## Overview
- Extract the hardcoded HTTP fetch logic from `extractWithRules` into a `Retriever` interface so different content retrieval backends can be swapped in
- Add `CloudflareRetriever` using Cloudflare Browser Rendering `/content` endpoint — returns fully rendered HTML after JS execution, handling sites behind bot protection or requiring JS rendering
- Addresses PR review feedback on radio-t/super-bot#156: the content extraction improvement belongs in ukeeper-readability (the extraction layer), not in super-bot (the consumer)

## Context (from discovery)
- **fetch logic**: embedded in `extractor/readability.go:extractWithRules` (lines 80-106) — creates `http.Client` inline, Safari user-agent, `io.ReadAll`, no abstraction
- **only existing interface**: `extractor.Rules` for datastore access; no fetcher interface exists
- **pipeline after fetch**: `toUtf8` → `getContent` (rules/readability) → title → `getText` → `normalizeLinks` → `getSnippet` → `extractPics` — all expects HTML input, stays unchanged
- **`normalizeLinks`** takes `*http.Request` but only uses `.URL` — will simplify to `*url.URL`
- **wiring**: `main.go` creates `extractor.UReadability` struct with `TimeOut`, `SnippetSize`, `Rules` fields; `rest.Server` holds it as a concrete struct
- **CF `/content` endpoint**: `POST /accounts/{id}/browser-rendering/content` with `{"url": "..."}`, Bearer token auth, returns rendered HTML

## Development Approach
- **testing approach**: Regular (code first, then tests)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change
- maintain backward compatibility — existing code creating `UReadability{}` without `Retriever` must continue to work (nil defaults to HTTP fetch)

## Testing Strategy
- **unit tests**: httptest mock servers for both retrievers, table-driven tests, testify assertions (matching existing patterns)
- **mock generation**: moq-generated mock for `Retriever` interface (same pattern as `Rules` mock)
- **integration**: existing `readability_test.go` tests must pass unchanged (they create `UReadability` without `Retriever`)

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with + prefix
- document issues/blockers with warning prefix
- update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Create Retriever interface and HTTPRetriever

**Files:**
- Create: `extractor/retriever.go`
- Create: `extractor/retriever_test.go`

- [x] define `Retriever` interface with `Retrieve(ctx context.Context, url string) (*RetrieveResult, error)` method
- [x] define `RetrieveResult` struct with `Body []byte`, `URL string`, `Header http.Header`
- [x] implement `HTTPRetriever` struct extracting the current fetch logic from `extractWithRules` (HTTP client, Safari user-agent, redirect following, body reading)
- [x] add `//go:generate moq` directive for `Retriever` interface
- [x] write tests for `HTTPRetriever`: successful fetch, redirect following, user-agent header, error cases (bad URL, connection refused)
- [x] run tests — must pass before next task

### Task 2: Implement CloudflareRetriever

**Files:**
- Modify: `extractor/retriever.go`
- Modify: `extractor/retriever_test.go`

- [x] implement `CloudflareRetriever` struct with `AccountID`, `APIToken`, `BaseURL` (for test override), `Timeout` fields
- [x] implement `Retrieve` method: POST to `/accounts/{id}/browser-rendering/content` with `{"url": "...", "gotoOptions": {"waitUntil": "networkidle0"}}`, Bearer token auth
- [x] handle response: try JSON `{"success": true, "result": "<html>"}` first, fall back to raw body; set `Content-Type: text/html; charset=utf-8` header
- [x] write tests: successful fetch (mock CF API), API error (non-200 status), JSON response format, raw HTML response format
- [x] run tests — must pass before next task

### Task 3: Wire Retriever into UReadability

**Files:**
- Modify: `extractor/readability.go`
- Modify: `extractor/readability_test.go`

- [x] add `Retriever Retriever` field to `UReadability` struct
- [x] add `retriever()` helper method: returns `f.Retriever` if non-nil, otherwise `&HTTPRetriever{Timeout: f.TimeOut}`
- [x] replace inline HTTP fetch in `extractWithRules` (lines 80-106) with `f.retriever().Retrieve(ctx, reqURL)` call
- [x] use `result.URL`, `result.Body`, `result.Header` instead of `resp.Request.URL`, `io.ReadAll(resp.Body)`, `resp.Header`
- [x] change `normalizeLinks` signature from `*http.Request` to `*url.URL` (only `.URL` field is used); update caller to pass parsed URL
- [x] remove unused imports from `readability.go` (`io`, `net/http`)
- [x] update `TestNormalizeLinks` and `TestNormalizeLinksIssue` to pass `*url.URL` instead of `&http.Request{URL: u}`
- [x] verify all existing tests pass unchanged (tests create `UReadability` without `Retriever` — nil defaults to HTTPRetriever)
- [x] run full test suite: `go test -timeout=60s -race ./...`

### Task 4: Add CLI flags and wiring in main.go

**Files:**
- Modify: `main.go`

- [ ] add `CFAccountID string` and `CFAPIToken string` fields to opts struct with `long`/`env` tags
- [ ] in `main()`, create `CloudflareRetriever` when both flags are set; log which retriever is active
- [ ] pass retriever to `UReadability` struct
- [ ] run full test suite: `go test -timeout=60s -race ./...`

### Task 5: Generate mock and run linter

**Files:**
- Create: `extractor/mocks/retriever.go` (generated)

- [ ] run `go generate ./extractor/...` to generate `Retriever` mock
- [ ] run `gofmt -w` on all modified files
- [ ] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [ ] fix any lint issues

### Task 6: Verify acceptance criteria

- [ ] verify `UReadability{}` without `Retriever` field works (backward compatible)
- [ ] verify `UReadability{Retriever: &CloudflareRetriever{...}}` works
- [ ] verify all existing tests pass: `go test -timeout=60s -race ./...`
- [ ] verify mock is generated and up to date

### Task 7: [Final] Update documentation

- [ ] update CLAUDE.md architecture section to mention `Retriever` interface
- [ ] update CLAUDE.md build section with new CLI flags
- [ ] move this plan to `docs/plans/completed/`

## Technical Details

### Retriever interface

```go
type Retriever interface {
    Retrieve(ctx context.Context, url string) (*RetrieveResult, error)
}

type RetrieveResult struct {
    Body   []byte      // raw page content (HTML)
    URL    string      // final URL after redirects
    Header http.Header // response headers (for charset detection)
}
```

### CloudflareRetriever request/response

```
POST https://api.cloudflare.com/client/v4/accounts/{account_id}/browser-rendering/content
Authorization: Bearer {api_token}
Content-Type: application/json

{"url": "https://example.com", "gotoOptions": {"waitUntil": "networkidle0"}}
```

Response: fully rendered HTML (may be JSON-wrapped `{"success": true, "result": "<html>"}` or raw HTML).

### CLI flags

| Flag | Env | Description |
|------|-----|-------------|
| `--cf-account-id` | `CF_ACCOUNT_ID` | Cloudflare account ID for Browser Rendering API |
| `--cf-api-token` | `CF_API_TOKEN` | Cloudflare API token with Browser Rendering Edit permission |

When both are set → `CloudflareRetriever`; otherwise → `HTTPRetriever` (default).

### Pipeline flow (unchanged)

```
Retriever.Retrieve(url)  →  toUtf8  →  getContent (rules/readability)  →  title
     ↑ NEW                     │              │
     │                         ↓              ↓
HTTPRetriever (default)    getText  →  normalizeLinks  →  getSnippet  →  extractPics
CloudflareRetriever (opt)
```

## Post-Completion

**External system updates:**
- super-bot deployment: add `CF_ACCOUNT_ID` and `CF_API_TOKEN` env vars to ukeeper-readability deployment config when switching to Cloudflare retrieval
- Cloudflare setup: create API token with "Browser Rendering - Edit" permission under the target account
- radio-t/super-bot#156: can be closed once this is deployed — super-bot continues using the existing `uKeeperGetter` interface unchanged

**Manual verification:**
- test against real Cloudflare Browser Rendering API with known problematic URLs (sites returning "just a moment..." to direct HTTP)
- verify free tier limits are acceptable (10 min/day browser time, 1 req/10 sec rate limit)
