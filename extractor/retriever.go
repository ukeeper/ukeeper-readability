package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

// Retrieve fetches the URL using an HTTP GET with Safari user-agent, following redirects
func (h *HTTPRetriever) Retrieve(ctx context.Context, reqURL string) (*RetrieveResult, error) {
	httpClient := &http.Client{Timeout: h.Timeout}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, http.NoBody)
	if err != nil {
		log.Printf("[WARN] failed to create request for %s, error=%v", reqURL, err)
		return nil, err
	}
	req.Close = true
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
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

// CloudflareRetriever fetches pages using Cloudflare Browser Rendering API.
// it sends a POST to the /content endpoint which returns fully rendered HTML after JS execution.
type CloudflareRetriever struct {
	AccountID string
	APIToken  string
	BaseURL   string // override for testing; defaults to Cloudflare API
	Timeout   time.Duration
}

// cfRequest is the request body for the Cloudflare Browser Rendering /content endpoint
type cfRequest struct {
	URL         string        `json:"url"`
	GotoOptions cfGotoOptions `json:"gotoOptions"`
}

// cfGotoOptions configures page navigation for Cloudflare Browser Rendering
type cfGotoOptions struct {
	WaitUntil string `json:"waitUntil"`
}

// cfResponse is the JSON response from the Cloudflare Browser Rendering /content endpoint
type cfResponse struct {
	Success bool   `json:"success"`
	Result  string `json:"result"`
}

// Retrieve fetches the URL via Cloudflare Browser Rendering /content endpoint
func (c *CloudflareRetriever) Retrieve(ctx context.Context, reqURL string) (*RetrieveResult, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}
	endpoint := fmt.Sprintf("%s/accounts/%s/browser-rendering/content", baseURL, c.AccountID)

	cfReq := cfRequest{
		URL:         reqURL,
		GotoOptions: cfGotoOptions{WaitUntil: "networkidle0"},
	}
	reqBody, err := json.Marshal(cfReq)
	if err != nil {
		return nil, fmt.Errorf("marshal cf request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create cf request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: c.Timeout}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[WARN] cloudflare request failed for %s, error=%v", reqURL, err)
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("[WARN] failed to close cf response body, error=%v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[WARN] failed to read cf response for %s, error=%v", reqURL, err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		bodySnippet := body
		if len(bodySnippet) > 512 {
			bodySnippet = bodySnippet[:512]
		}
		return nil, fmt.Errorf("cloudflare API error: status %d, body: %s", resp.StatusCode, string(bodySnippet))
	}

	// try JSON response format first: {"success": true, "result": "<html>"}
	var cfResp cfResponse
	if err = json.Unmarshal(body, &cfResp); err == nil {
		switch {
		case cfResp.Success && cfResp.Result != "":
			body = []byte(cfResp.Result)
		case cfResp.Success && cfResp.Result == "":
			return nil, fmt.Errorf("cloudflare returned empty content for %s", reqURL)
		default: // !cfResp.Success
			return nil, fmt.Errorf("cloudflare API returned success=false for %s", reqURL)
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
	}, nil
}
