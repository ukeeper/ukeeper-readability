package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
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
