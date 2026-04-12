package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	log "github.com/go-pkgz/lgr"
)

//go:generate moq -out retriever_mock_test.go -skip-ensure -fmt goimports . Retriever

// Retriever abstracts how page content is fetched from a URL
type Retriever interface {
	Retrieve(ctx context.Context, url string) (*RetrieveResult, error)
}

// RetrieveResult holds the raw page content and metadata from a retrieval
type RetrieveResult struct {
	Body   []byte      // raw page content (HTML)
	URL    string      // final URL after redirects
	Header http.Header // response headers (for charset detection)
}

// HTTPRetriever fetches pages using a standard HTTP client
type HTTPRetriever struct {
	Timeout time.Duration

	once   sync.Once
	client *http.Client
}

const httpDefaultTimeout = 30 * time.Second

func (h *HTTPRetriever) httpClient() *http.Client {
	h.once.Do(func() {
		timeout := h.Timeout
		if timeout == 0 {
			timeout = httpDefaultTimeout
		}
		h.client = &http.Client{Timeout: timeout}
	})
	return h.client
}

// Retrieve fetches the URL using an HTTP GET with Safari user-agent, following redirects
func (h *HTTPRetriever) Retrieve(ctx context.Context, reqURL string) (*RetrieveResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, http.NoBody)
	if err != nil {
		log.Printf("[WARN] failed to create request for %s, error=%v", reqURL, err)
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := h.httpClient().Do(req)
	if err != nil {
		log.Printf("[WARN] failed to get anything from %s, error=%v", reqURL, err)
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("[WARN] failed to close response body, error=%v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[WARN] failed to read data from %s, error=%v", reqURL, err)
		return nil, err
	}

	return &RetrieveResult{
		Body:   body,
		URL:    resp.Request.URL.String(),
		Header: resp.Header,
	}, nil
}

const (
	cfDefaultBaseURL    = "https://api.cloudflare.com/client/v4"
	cfDefaultWaitUntil  = "networkidle0"
	cfDefaultTimeout    = 60 * time.Second
	cfDefaultRetryDelay = 11 * time.Second // free tier: 1 req / 10s — add a little headroom
	cfMaxRetryDelay     = 30 * time.Second

	// CFDefaultMaxRetries is the suggested MaxRetries value for production CloudflareRetriever setup.
	// 2 retries keeps worst-case backoff at ~33s (11s + 22s) so the total handler time stays under
	// common upstream timeouts (nginx proxy_read_timeout default is 60s). Callers must set MaxRetries
	// explicitly — CloudflareRetriever does not substitute a default for the zero value.
	CFDefaultMaxRetries = 2
)

// errCFRateLimited is returned by the single-attempt inner retrieve when the CF API signals rate limiting;
// the outer Retrieve uses it to decide whether to back off and retry.
var errCFRateLimited = errors.New("cloudflare rate limited")

// CloudflareRetriever fetches pages using Cloudflare Browser Rendering API.
// it sends a POST to the /content endpoint which returns fully rendered HTML after JS execution.
// on HTTP 429 it retries with backoff (respecting Retry-After) up to MaxRetries times.
type CloudflareRetriever struct {
	AccountID  string
	APIToken   string
	BaseURL    string        // override for testing; defaults to Cloudflare API
	Timeout    time.Duration // per-request HTTP client timeout; defaults to 60s
	MaxRetries int           // number of retries on 429; defaults to 3. set to -1 to disable retries
	RetryDelay time.Duration // base delay between 429 retries; defaults to 11s (CF free tier is 1 req/10s)

	once   sync.Once
	client *http.Client
}

func (c *CloudflareRetriever) httpClient() *http.Client {
	c.once.Do(func() {
		timeout := c.Timeout
		if timeout == 0 {
			timeout = cfDefaultTimeout
		}
		c.client = &http.Client{Timeout: timeout}
	})
	return c.client
}

type cfRequest struct {
	URL         string        `json:"url"`
	GotoOptions cfGotoOptions `json:"gotoOptions"`
}

type cfGotoOptions struct {
	WaitUntil string `json:"waitUntil"`
}

type cfResponse struct {
	Success bool   `json:"success"`
	Result  string `json:"result"`
}

// Retrieve fetches the URL via Cloudflare Browser Rendering /content endpoint.
// on HTTP 429 it backs off and retries up to MaxRetries times, holding the caller's
// connection open in the meantime. aborts early if the caller's context is canceled.
// MaxRetries: 0 means no retries. RetryDelay: 0 falls back to the package default.
func (c *CloudflareRetriever) Retrieve(ctx context.Context, reqURL string) (*RetrieveResult, error) {
	maxRetries := c.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	baseDelay := c.RetryDelay
	if baseDelay <= 0 {
		baseDelay = cfDefaultRetryDelay
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, retryAfter, err := c.doRetrieve(ctx, reqURL)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !errors.Is(err, errCFRateLimited) {
			return nil, err
		}
		if attempt == maxRetries {
			break
		}
		delay := retryAfter
		if delay <= 0 {
			delay = baseDelay << attempt // 11s, 22s, 44s, ...
		}
		if delay > cfMaxRetryDelay {
			delay = cfMaxRetryDelay
		}
		log.Printf("[INFO] cloudflare rate limited for %s, retry %d/%d after %s", reqURL, attempt+1, maxRetries, delay)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

// doRetrieve performs a single Browser Rendering request. on 429 it returns errCFRateLimited
// (possibly wrapped) and the parsed Retry-After duration (0 if absent or unparseable).
func (c *CloudflareRetriever) doRetrieve(ctx context.Context, reqURL string) (*RetrieveResult, time.Duration, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = cfDefaultBaseURL
	}
	endpoint := fmt.Sprintf("%s/accounts/%s/browser-rendering/content", baseURL, c.AccountID)

	cfReq := cfRequest{
		URL:         reqURL,
		GotoOptions: cfGotoOptions{WaitUntil: cfDefaultWaitUntil},
	}
	reqBody, err := json.Marshal(cfReq)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal cf request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, fmt.Errorf("create cf request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		log.Printf("[WARN] cloudflare request failed for %s, error=%v", reqURL, err)
		return nil, 0, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("[WARN] failed to close cf response body, error=%v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[WARN] failed to read cf response for %s, error=%v", reqURL, err)
		return nil, 0, err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, retryAfter, fmt.Errorf("%w: status 429", errCFRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		bodySnippet := body
		if len(bodySnippet) > 512 {
			bodySnippet = bodySnippet[:512]
		}
		return nil, 0, fmt.Errorf("cloudflare API error: status %d, body: %s", resp.StatusCode, string(bodySnippet))
	}

	// try JSON response format first: {"success": true, "result": "<html>"}
	var cfResp cfResponse
	if err = json.Unmarshal(body, &cfResp); err == nil {
		switch {
		case cfResp.Success && cfResp.Result != "":
			body = []byte(cfResp.Result)
		case cfResp.Success && cfResp.Result == "":
			return nil, 0, fmt.Errorf("cloudflare returned empty content for %s", reqURL)
		default: // !cfResp.Success
			return nil, 0, fmt.Errorf("cloudflare API returned success=false for %s", reqURL)
		}
	}
	// if unmarshal fails, use the raw body as-is (raw HTML response)

	header := make(http.Header)
	header.Set("Content-Type", "text/html; charset=utf-8")

	// note: URL is the original request URL, not the final URL after any JS-driven redirects,
	// because the Cloudflare Browser Rendering /content API does not expose the final navigated URL
	return &RetrieveResult{
		Body:   body,
		URL:    reqURL,
		Header: header,
	}, 0, nil
}

// parseRetryAfter parses an HTTP Retry-After header value as either delta-seconds
// or an HTTP date. returns 0 if empty or unparseable.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		if delta := time.Until(t); delta > 0 {
			return delta
		}
	}
	return 0
}
