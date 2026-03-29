# Auto-improving Content Extraction with OpenAI Evaluation

## Overview
- Add automatic extraction quality evaluation using OpenAI during content extraction
- When OpenAI is configured and no existing rule for the domain, GPT evaluates the extraction result and suggests CSS selectors if the result is poor
- Iterates up to 3 times (configurable), saves the best selector as a rule for future use
- `ExtractAndImprove()` force mode for when a user says "this URL is bad, fix the rules" — ignores existing rules, re-extracts with general parser, and re-evaluates
- Protected REST endpoint `GET /api/content-parsed-wrong?url=...` for force mode
- Independent from PR #32 (summary feature) — shares config namespace for future compatibility
- Fail-open: GPT errors never break extraction

## Context (from discovery)
- **base branch**: `modularise-retrieval` (PR #73) — has `Retriever` interface for HTTP fetching
- **extraction pipeline**: `extractor/readability.go` — `Extract()` → `extractWithRules()` → fetch via Retriever → charset → rules/readability → post-processing
- **rules system**: `extractor.Rules` interface, `datastore.RulesDAO` — one rule per domain, keyed on `domain`
- **existing interfaces**: `Rules`, `Retriever` in extractor package, mocks in `extractor/mocks/`
- **REST routes**: protected group uses `basicAuth` middleware in `rest/server.go`
- **config pattern**: `jessevdk/go-flags` with `long`/`env` tags in `opts` struct
- **pointer receivers**: `UReadability` methods use pointer receivers; `rest.Server.Readability` is stored by value but Go auto-addresses for pointer receiver calls — no change needed

## Development Approach
- **testing approach**: Regular (code first, then tests)
- complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- maintain backward compatibility — `Extract()` without OpenAI configured works exactly as before

## Testing Strategy
- **unit tests**: mock `AIEvaluator` interface for GPT interactions, mock `Rules` for rule saving
- **table-driven tests**: cover good extraction (bail early), bad extraction (iterate), GPT errors (fail open), force mode
- **existing tests**: must pass unchanged — `Extract()` without OpenAI config is unaffected

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with + prefix
- document issues/blockers with warning prefix

## Implementation Steps

### Task 1: Add go-openai dependency, AIEvaluator interface, and OpenAI implementation

**Files:**
- Modify: `go.mod`, `go.sum`, `vendor/`
- Create: `extractor/evaluator.go`
- Create: `extractor/evaluator_test.go`
- Create: `extractor/mocks/evaluator.go` (generated)

- [ ] run `go get github.com/sashabaranov/go-openai@latest && go mod tidy && go mod vendor`
- [ ] define `AIEvaluator` interface with `Evaluate(ctx, url, extractedText, htmlBody string) (*EvalResult, error)` method
- [ ] define `EvalResult` struct: `Good bool`, `Selector string`
- [ ] implement `OpenAIEvaluator` struct with `APIKey`, `Model` fields
- [ ] implement `Evaluate` method: build prompt with URL + extracted text (first 2000 chars) + truncated HTML body (first 4000 chars), parse JSON response `{"good": true}` or `{"good": false, "selector": "..."}`
- [ ] handle invalid JSON response: retry once, then return `EvalResult{Good: true}` (fail open)
- [ ] add `//go:generate moq` directive for `AIEvaluator`, run `go generate` to create mock
- [ ] write tests: successful good evaluation, successful bad evaluation with selector, invalid JSON response, OpenAI API error
- [ ] run tests — must pass before next task

### Task 2: Wire AIEvaluator into UReadability and add evaluation loop

**Files:**
- Modify: `extractor/readability.go`
- Modify: `extractor/readability_test.go`

- [ ] add `AIEvaluator AIEvaluator` and `MaxGPTIter int` fields to `UReadability` struct
- [ ] change `extractWithRules` signature to `extractWithRules(ctx, reqURL string, rule *datastore.Rule, force bool)`
- [ ] update callers: `Extract()` passes `force=false`, `ExtractByRule()` passes `force=false`
- [ ] add `ExtractAndImprove(ctx, url)` public method — calls `extractWithRules(ctx, url, nil, true)`
- [ ] add `evaluateAndImprove(ctx, reqURL, htmlBody string, result *Response) *Response` private method
- [ ] implement evaluation loop: up to `MaxGPTIter` iterations (default 3); send URL + result.Content + htmlBody to evaluator; try suggested selector on htmlBody via goquery; feed new extraction back to GPT on next iteration; if GPT says good, break
- [ ] in `extractWithRules`: after extraction, call `evaluateAndImprove` if: `AIEvaluator != nil` AND (`force` OR no existing rule for domain)
- [ ] **force mode semantics**: when `force=true`, pass `nil` as rule to `getContent()` so initial extraction uses the general parser (not the stored rule), then let GPT suggest a new selector
- [ ] if better selector found, save rule via `f.Rules.Save()` with domain and selector
- [ ] all GPT/evaluation errors logged and swallowed — original result returned unchanged
- [ ] write tests: extraction with evaluator (good on first try), extraction with evaluator (bad, improved on retry), extraction without evaluator (unchanged behaviour), GPT error (fail open), force mode ignores existing rules and extracts with general parser
- [ ] run tests — must pass before next task

### Task 3: Add CLI flags and wiring in main.go

**Files:**
- Modify: `main.go`

- [ ] add `OpenAIKey string` field (`--openai-api-key` / `OPENAI_API_KEY`)
- [ ] add `OpenAIModel string` field (`--openai-model` / `OPENAI_MODEL` default `gpt-5.4-mini`)
- [ ] add `MaxGPTIter int` field (`--openai-max-iter` / `OPENAI_MAX_ITER` default `3`)
- [ ] when `OpenAIKey` is set, create `OpenAIEvaluator` and inject into `UReadability`
- [ ] log which mode is active (with/without OpenAI evaluation)
- [ ] run tests — must pass before next task

### Task 4: Add REST endpoint for force mode

**Files:**
- Modify: `rest/server.go`
- Modify: `rest/server_test.go`

- [ ] add `GET /content-parsed-wrong` route in the protected group within `api.Mount("/api")` (full path: `/api/content-parsed-wrong`, requires basicAuth)
- [ ] implement `contentParsedWrong` handler: validate `url` query param, check `AIEvaluator` is configured, call `s.Readability.ExtractAndImprove()`, return JSON result
- [ ] write tests: successful call, missing url param, missing OpenAI config (AIEvaluator nil)
- [ ] run tests — must pass before next task

### Task 5: Run linter and final checks

- [ ] run `gofmt -w` on all modified files
- [ ] run `go fix ./...`
- [ ] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [ ] fix any lint issues
- [ ] run tests — must pass before next task

### Task 6: Verify acceptance criteria

- [ ] verify `Extract()` without OpenAI configured works exactly as before (existing tests pass)
- [ ] verify `Extract()` with OpenAI configured evaluates and improves extraction (test with mock evaluator)
- [ ] verify `Extract()` skips evaluation when domain already has a rule (test with mock rules returning a rule)
- [ ] verify `ExtractAndImprove()` runs evaluation even when rule exists, using general parser for initial extraction
- [ ] verify GPT errors don't break extraction (test with evaluator returning error)
- [ ] verify rule is saved when better selector found (test with mock rules verifying Save call)
- [ ] run full test suite: `go test -timeout=60s -race ./...`

### Task 7: [Final] Update documentation

- [ ] update README.md with OpenAI configuration flags
- [ ] update CLAUDE.md with new AIEvaluator interface and extraction flow
- [ ] move this plan to `docs/plans/completed/`

## Technical Details

### AIEvaluator interface

```go
type AIEvaluator interface {
    Evaluate(ctx context.Context, url, extractedText, htmlBody string) (*EvalResult, error)
}

type EvalResult struct {
    Good     bool   // true if extraction looks fine
    Selector string // suggested CSS selector (only when Good=false)
}
```

### GPT prompt

```
System: You are a web content extraction expert. You evaluate whether
extracted article text is complete and correct, and suggest CSS selectors
when extraction is poor.

User: I extracted content from this URL: {url}

Extracted text (first 2000 chars):
---
{extracted_text}
---

Page HTML structure (first 4000 chars):
---
{html_body}
---

Is this a good extraction of the article content? Consider:
- Does it contain the main article body (not just navigation/ads/boilerplate)?
- Is it reasonably complete (not truncated or empty)?

Respond in JSON only, no other text:
{"good": true} if extraction is fine
{"good": false, "selector": "article.post-content"} if not, with a CSS selector
that targets the main content on this page
```

On subsequent iterations, append:
```
Previous attempt with selector "{prev_selector}" was tried but didn't improve.
Suggest a different selector based on the HTML structure above.
```

### Evaluation loop flow

```
Extract(ctx, url)       → extractWithRules(ctx, url, nil, force=false)
ExtractAndImprove(ctx, url) → extractWithRules(ctx, url, nil, force=true)
ExtractByRule(ctx, url, rule) → extractWithRules(ctx, url, rule, force=false)

extractWithRules(ctx, url, rule, force):
  ├─ fetch HTML via Retriever → htmlBody
  ├─ if force → getContent(ctx, body, url, nil)   // ignore stored rule
  │  else     → getContent(ctx, body, url, rule)   // normal: use rule if provided/stored
  ├─ post-processing (title, links, images, snippet)
  │
  ├─ if AIEvaluator == nil → return result
  ├─ if !force && domain has existing rule → return result
  │
  └─ evaluateAndImprove(ctx, url, htmlBody, result):
      best = result
      for i := 0; i < MaxGPTIter; i++:
        eval, err = AIEvaluator.Evaluate(ctx, url, best.Content, htmlBody)
        if err != nil → log, return best (fail open)
        if eval.Good → break (extraction is fine)
        newContent = goquery.Find(eval.Selector) on htmlBody
        if newContent is empty → continue (selector didn't match)
        best = rebuild Response with newContent
      if best changed → Rules.Save(domain, selector used for best)
      return best
```

### CLI flags

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--openai-api-key` | `OPENAI_API_KEY` | none | OpenAI API key; enables auto-evaluation when set |
| `--openai-model` | `OPENAI_MODEL` | `gpt-5.4-mini` | OpenAI model for evaluation (no validation, any model string accepted) |
| `--openai-max-iter` | `OPENAI_MAX_ITER` | `3` | Max evaluation iterations per extraction |

### Config compatibility with PR #32

When PR #32 (summary feature) eventually merges, the flags can be consolidated:
- `--openai-api-key` stays the same
- `--openai-model` stays the same
- Summary-specific flags get added separately

## Post-Completion

**Manual verification:**
- test with a known poorly-extracted URL to verify the loop creates a working rule
- test that subsequent requests for the same domain use the saved rule (no GPT call)
- test force mode via the REST endpoint

**External system updates:**
- deployment: add `OPENAI_API_KEY` env var when ready to enable
- super-bot: no changes needed (continues using the same extraction API)
