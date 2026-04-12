package extractor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPRetriever_Retrieve(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			assert.Equal(t, userAgent, r.Header.Get("User-Agent"), "should send Safari user-agent")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><body>hello</body></html>"))
		case "/redirect-start":
			http.Redirect(w, r, "/redirect-end", http.StatusFound)
		case "/redirect-end":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>redirected</body></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	tests := []struct {
		name     string
		url      string
		wantBody string
		wantURL  string
		wantErr  bool
	}{
		{
			name:     "successful fetch",
			url:      ts.URL + "/ok",
			wantBody: "<html><body>hello</body></html>",
			wantURL:  ts.URL + "/ok",
		},
		{
			name:     "redirect following",
			url:      ts.URL + "/redirect-start",
			wantBody: "<html><body>redirected</body></html>",
			wantURL:  ts.URL + "/redirect-end",
		},
		{
			name:    "bad url",
			url:     "http://\x00invalid",
			wantErr: true,
		},
		{
			name:    "connection refused",
			url:     "http://127.0.0.1:1",
			wantErr: true,
		},
	}

	retriever := &HTTPRetriever{Timeout: 5 * time.Second}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := retriever.Retrieve(context.Background(), tt.url)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBody, string(result.Body))
			assert.Equal(t, tt.wantURL, result.URL)
			assert.NotNil(t, result.Header)
		})
	}
}

func TestHTTPRetriever_UserAgent(t *testing.T) {
	var receivedUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	retriever := &HTTPRetriever{Timeout: 5 * time.Second}
	_, err := retriever.Retrieve(context.Background(), ts.URL)
	require.NoError(t, err)
	assert.Equal(t, userAgent, receivedUA)
}

func TestHTTPRetriever_ResponseHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=windows-1251")
		w.Header().Set("X-Custom", "test-value")
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	retriever := &HTTPRetriever{Timeout: 5 * time.Second}
	result, err := retriever.Retrieve(context.Background(), ts.URL)
	require.NoError(t, err)
	assert.Equal(t, "text/html; charset=windows-1251", result.Header.Get("Content-Type"))
	assert.Equal(t, "test-value", result.Header.Get("X-Custom"))
}

func TestCloudflareRetriever_Retrieve(t *testing.T) {
	const testHTML = "<html><body>rendered content</body></html>"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/accounts/test-account/browser-rendering/content", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// verify request body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var req cfRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "networkidle0", req.GotoOptions.WaitUntil)

		switch req.URL {
		case "https://example.com/json-response":
			w.Header().Set("Content-Type", "application/json")
			resp := cfResponse{Success: true, Result: testHTML}
			_ = json.NewEncoder(w).Encode(resp)
		case "https://example.com/raw-html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(testHTML))
		case "https://example.com/error":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"forbidden"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	tests := []struct {
		name     string
		url      string
		wantBody string
		wantURL  string
		wantErr  string
	}{
		{
			name:     "successful fetch with JSON response",
			url:      "https://example.com/json-response",
			wantBody: testHTML,
			wantURL:  "https://example.com/json-response",
		},
		{
			name:     "successful fetch with raw HTML response",
			url:      "https://example.com/raw-html",
			wantBody: testHTML,
			wantURL:  "https://example.com/raw-html",
		},
		{
			name:    "API error returns error",
			url:     "https://example.com/error",
			wantErr: "cloudflare API error: status 403",
		},
	}

	retriever := &CloudflareRetriever{
		AccountID: "test-account",
		APIToken:  "test-token",
		BaseURL:   ts.URL,
		Timeout:   5 * time.Second,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := retriever.Retrieve(context.Background(), tt.url)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBody, string(result.Body))
			assert.Equal(t, tt.wantURL, result.URL)
			assert.Equal(t, "text/html; charset=utf-8", result.Header.Get("Content-Type"))
		})
	}
}

func TestCloudflareRetriever_URLPathConstruction(t *testing.T) {
	// verify that the retriever constructs the correct Cloudflare API URL path from AccountID
	var capturedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer ts.Close()

	retriever := &CloudflareRetriever{
		AccountID: "my-account",
		APIToken:  "my-token",
		BaseURL:   ts.URL,
		Timeout:   5 * time.Second,
	}
	_, err := retriever.Retrieve(context.Background(), "https://example.com")
	require.Error(t, err)
	assert.Equal(t, "/accounts/my-account/browser-rendering/content", capturedPath)
}

func TestCloudflareRetriever_SuccessEmptyResult(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":""}`))
	}))
	defer ts.Close()

	retriever := &CloudflareRetriever{
		AccountID: "test-account",
		APIToken:  "test-token",
		BaseURL:   ts.URL,
		Timeout:   5 * time.Second,
	}
	_, err := retriever.Retrieve(context.Background(), "https://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty content")
}

func TestCloudflareRetriever_SuccessFalse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"rate limited"}]}`))
	}))
	defer ts.Close()

	retriever := &CloudflareRetriever{
		AccountID: "test-account",
		APIToken:  "test-token",
		BaseURL:   ts.URL,
		Timeout:   5 * time.Second,
	}
	_, err := retriever.Retrieve(context.Background(), "https://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "success=false")
}

func TestCloudflareRetriever_RateLimitThenSuccess(t *testing.T) {
	const testHTML = "<html><body>recovered</body></html>"
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":2001,"message":"Rate limit exceeded"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfResponse{Success: true, Result: testHTML})
	}))
	defer ts.Close()

	retriever := &CloudflareRetriever{
		AccountID:  "test-account",
		APIToken:   "test-token",
		BaseURL:    ts.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond, // fast in tests
	}
	result, err := retriever.Retrieve(context.Background(), "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, testHTML, string(result.Body))
	assert.Equal(t, int32(3), calls.Load(), "should have retried twice before succeeding")
}

func TestCloudflareRetriever_RateLimitExhausted(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":2001,"message":"Rate limit exceeded"}]}`))
	}))
	defer ts.Close()

	retriever := &CloudflareRetriever{
		AccountID:  "test-account",
		APIToken:   "test-token",
		BaseURL:    ts.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	}
	_, err := retriever.Retrieve(context.Background(), "https://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
	assert.Equal(t, int32(3), calls.Load(), "initial attempt + 2 retries")
}

func TestCloudflareRetriever_RateLimitHonoursRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var timestamps [3]time.Time
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if int(n) <= len(timestamps) {
			timestamps[n-1] = time.Now()
		}
		if n < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`rate limited`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfResponse{Success: true, Result: "<html>ok</html>"})
	}))
	defer ts.Close()

	retriever := &CloudflareRetriever{
		AccountID:  "test-account",
		APIToken:   "test-token",
		BaseURL:    ts.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
		RetryDelay: 10 * time.Second, // would be ignored in favour of Retry-After
	}
	start := time.Now()
	_, err := retriever.Retrieve(context.Background(), "https://example.com")
	elapsed := time.Since(start)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond, "should have waited for Retry-After")
	assert.Less(t, elapsed, 3*time.Second, "should have used 1s Retry-After, not 10s base delay")
}

func TestCloudflareRetriever_RateLimitContextCancelled(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`rate limited`))
	}))
	defer ts.Close()

	retriever := &CloudflareRetriever{
		AccountID:  "test-account",
		APIToken:   "test-token",
		BaseURL:    ts.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 5,
		RetryDelay: 10 * time.Second, // long enough that ctx cancel will fire first
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := retriever.Retrieve(ctx, "https://example.com")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(1), calls.Load(), "should abort in backoff before second call")
}

func TestCloudflareRetriever_RateLimitRetriesDisabled(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
	}{
		{name: "zero means no retries", maxRetries: 0},
		{name: "negative clamped to zero", maxRetries: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`rate limited`))
			}))
			defer ts.Close()

			retriever := &CloudflareRetriever{
				AccountID:  "test-account",
				APIToken:   "test-token",
				BaseURL:    ts.URL,
				Timeout:    5 * time.Second,
				MaxRetries: tt.maxRetries,
			}
			_, err := retriever.Retrieve(context.Background(), "https://example.com")
			require.Error(t, err)
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
		// allow a bit of slack for HTTP-date cases where time.Until is evaluated at assertion time
		tolerance time.Duration
	}{
		{name: "empty", value: "", want: 0},
		{name: "seconds", value: "5", want: 5 * time.Second},
		{name: "zero", value: "0", want: 0},
		{name: "negative", value: "-5", want: 0},
		{name: "garbage", value: "soon", want: 0},
		{name: "http date in the past", value: "Mon, 02 Jan 2006 15:04:05 GMT", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRetryAfter(tt.value)
			if tt.tolerance == 0 {
				assert.Equal(t, tt.want, got)
				return
			}
			assert.InDelta(t, tt.want, got, float64(tt.tolerance))
		})
	}
}
