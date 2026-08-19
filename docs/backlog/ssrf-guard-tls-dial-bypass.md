---
worth: yes
where: extractor/pics.go:24
added: 2026-08-19
---
# the SSRF guard in PR #83 can be bypassed for HTTPS

PR #83 ("Block SSRF to non-public addresses via connect-time IP guard") builds `safeTransport` by
cloning `http.DefaultTransport` and replacing `DialContext` with a guarded dialer. It leaves
`DialTLS` and `DialTLSContext` on the clone untouched.

Per the `net/http` contract a non-nil `DialTLSContext` is used for non-proxied HTTPS requests and
`DialContext` is not called at all. So for any transport where those hooks are set, every HTTPS
fetch skips the guard entirely while the code still advertises the protection. The stock
`http.DefaultTransport` has both nil, which is why the PR's tests pass and nothing shows up today.
The exposure appears as soon as a consumer, an instrumentation library, or anything else installs a
customised `*http.Transport`.

Fix is to clear both TLS dial hooks on the clone after cloning, so TLS connections resolve through
the guarded `DialContext` like everything else.

The rest of the design is sound and worth keeping: `net.Dialer.Control` sees the resolved socket
address, so the check runs per connection and therefore survives redirects and DNS rebinding, the
guarded transport disables proxies, and the classifier covers private, loopback, link-local,
multicast, unspecified, CGNAT, metadata and reserved ranges.

Separately, #83 is conflicting: #85 (`de6d22b`) rewrote `extractor/pics.go` for streaming, request
context, an overall budget and bounded concurrency, and added the shared `imageClient()` at line 24.
A rebase has to preserve all of that and attach the guarded transport to that existing shared client
rather than reintroducing the old one. Reviewing the pre-rebase merge result is not trustworthy.

Found by codex while reviewing #83 on 2026-08-19; not raised on the PR at the time. The `where`
above points at the shared client the guard must attach to, since `extractor/safedial.go` exists
only on the PR branch.
