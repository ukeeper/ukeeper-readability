package extractor

import (
	"context"
	"io"
	"net/http"
	"time"

	log "github.com/go-pkgz/lgr"
)

//go:generate moq -out mocks/retriever.go -pkg mocks -skip-ensure -fmt goimports . Retriever

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
		if err = resp.Body.Close(); err != nil {
			log.Printf("[WARN] failed to close response body, error=%v", err)
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
