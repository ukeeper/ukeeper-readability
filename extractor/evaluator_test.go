package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEvalResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		wantGood bool
		wantSel  string
	}{
		{"good extraction", `{"good": true}`, false, true, ""},
		{"bad extraction with selector", `{"good": false, "selector": "div.article-body"}`, false, false, "div.article-body"},
		{"invalid json", `not json at all`, true, false, ""},
		{"empty string", ``, true, false, ""},
		{"partial json", `{"good": `, true, false, ""},
		{"good false no selector", `{"good": false}`, false, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseEvalResponse(tt.input)
			if tt.wantErr {
				assert.ErrorIs(t, err, errInvalidJSON)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantGood, result.Good)
			assert.Equal(t, tt.wantSel, result.Selector)
		})
	}
}

func TestBuildUserPrompt(t *testing.T) {
	t.Run("basic prompt", func(t *testing.T) {
		prompt := buildUserPrompt("https://example.com/article", "some text", "<html>body</html>", "")
		assert.Contains(t, prompt, "https://example.com/article")
		assert.Contains(t, prompt, "some text")
		assert.Contains(t, prompt, "<html>body</html>")
		assert.NotContains(t, prompt, "Previous attempt")
	})

	t.Run("with previous selector", func(t *testing.T) {
		prompt := buildUserPrompt("https://example.com", "text", "<html/>", "div.content")
		assert.Contains(t, prompt, `Previous attempt with selector "div.content"`)
		assert.Contains(t, prompt, "Suggest a different selector")
	})

	t.Run("truncates long text", func(t *testing.T) {
		longText := strings.Repeat("a", 5000)
		longHTML := strings.Repeat("b", 8000)
		prompt := buildUserPrompt("https://example.com", longText, longHTML, "")
		// the prompt should not contain the full strings
		assert.NotContains(t, prompt, strings.Repeat("a", 5000))
		assert.NotContains(t, prompt, strings.Repeat("b", 8000))
		// but should contain the truncated versions
		assert.Contains(t, prompt, strings.Repeat("a", maxExtractedTextLen))
		assert.Contains(t, prompt, strings.Repeat("b", maxHTMLBodyLen))
	})
}

func TestOpenAIEvaluator_Evaluate(t *testing.T) {
	t.Run("good evaluation", func(t *testing.T) {
		ts := newOpenAIMockServer(t, `{"good": true}`)
		defer ts.Close()

		eval := newTestEvaluator(ts)
		result, err := eval.Evaluate(context.Background(), "https://example.com", "article text", "<html/>", "")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Good)
		assert.Empty(t, result.Selector)
	})

	t.Run("bad evaluation with selector", func(t *testing.T) {
		ts := newOpenAIMockServer(t, `{"good": false, "selector": "article.main"}`)
		defer ts.Close()

		eval := newTestEvaluator(ts)
		result, err := eval.Evaluate(context.Background(), "https://example.com", "nav links", "<html/>", "")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Good)
		assert.Equal(t, "article.main", result.Selector)
	})

	t.Run("invalid json response retries then fails", func(t *testing.T) {
		callCount := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			resp := openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "I'm not JSON"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck
		}))
		defer ts.Close()

		eval := newTestEvaluator(ts)
		result, err := eval.Evaluate(context.Background(), "https://example.com", "text", "<html/>", "")
		assert.ErrorIs(t, err, errInvalidJSON)
		assert.Nil(t, result)
		assert.Equal(t, 2, callCount, "should have retried once")
	})

	t.Run("invalid json then valid on retry", func(t *testing.T) {
		callCount := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			var content string
			if callCount == 1 {
				content = "not valid json"
			} else {
				content = `{"good": false, "selector": "div.main"}`
			}
			resp := openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: content}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck
		}))
		defer ts.Close()

		eval := newTestEvaluator(ts)
		result, err := eval.Evaluate(context.Background(), "https://example.com", "text", "<html/>", "")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Good)
		assert.Equal(t, "div.main", result.Selector)
		assert.Equal(t, 2, callCount, "should have retried once")
	})

	t.Run("api error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error": {"message": "server error"}}`)
		}))
		defer ts.Close()

		eval := newTestEvaluator(ts)
		result, err := eval.Evaluate(context.Background(), "https://example.com", "text", "<html/>", "")
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("empty choices", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck
		}))
		defer ts.Close()

		eval := newTestEvaluator(ts)
		result, err := eval.Evaluate(context.Background(), "https://example.com", "text", "<html/>", "")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no choices")
	})
}

// newOpenAIMockServer creates a test server that returns the given content as an OpenAI response
func newOpenAIMockServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: content}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
}

// newTestEvaluator creates an OpenAIEvaluator pointing at a test server
func newTestEvaluator(ts *httptest.Server) *OpenAIEvaluator {
	config := openai.DefaultConfig("test-key")
	config.BaseURL = ts.URL + "/v1"
	eval := &OpenAIEvaluator{APIKey: "test-key", Model: "test-model"}
	eval.clientConfig = &config
	return eval
}
