package extractor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/go-pkgz/lgr"
	openai "github.com/sashabaranov/go-openai"
)

//go:generate moq -out evaluator_mock_test.go -skip-ensure -fmt goimports . AIEvaluator

// AIEvaluator evaluates extraction quality and suggests CSS selectors for improvement
type AIEvaluator interface {
	Evaluate(ctx context.Context, url, extractedText, htmlBody, prevSelector string) (*EvalResult, error)
}

// EvalResult holds the evaluation outcome from the AI model
type EvalResult struct {
	Good     bool   // true if extraction looks fine
	Selector string // suggested CSS selector (only when Good=false)
}

const (
	maxExtractedTextLen = 2000
	maxHTMLBodyLen      = 4000
	openaiCallTimeout   = 60 * time.Second
)

var errInvalidJSON = errors.New("invalid JSON response from OpenAI")

const systemPrompt = `You are a web content extraction expert. You evaluate whether extracted article text is complete and correct, and suggest CSS selectors when extraction is poor.`

// OpenAIEvaluator uses OpenAI API to evaluate extraction quality
type OpenAIEvaluator struct {
	APIKey       string
	Model        string
	clientConfig *openai.ClientConfig // optional, for testing
	clientOnce   sync.Once
	client       *openai.Client
}

// getClient returns the OpenAI client, creating it once on first use
func (e *OpenAIEvaluator) getClient() *openai.Client {
	e.clientOnce.Do(func() {
		if e.clientConfig != nil {
			e.client = openai.NewClientWithConfig(*e.clientConfig)
		} else {
			e.client = openai.NewClient(e.APIKey)
		}
	})
	return e.client
}

// Evaluate sends the extracted text and HTML body to OpenAI for evaluation.
// Returns EvalResult indicating whether extraction is good, or suggests a CSS selector.
func (e *OpenAIEvaluator) Evaluate(ctx context.Context, reqURL, extractedText, htmlBody, prevSelector string) (*EvalResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, openaiCallTimeout)
	defer cancel()

	client := e.getClient()
	userPrompt := buildUserPrompt(reqURL, extractedText, htmlBody, prevSelector)

	result, err := e.callAPI(callCtx, client, userPrompt)
	if err != nil {
		if !errors.Is(err, errInvalidJSON) {
			return nil, err
		}
		// retry once on invalid JSON
		log.Printf("[WARN] invalid JSON from OpenAI for %s, retrying once", reqURL)
		result, err = e.callAPI(callCtx, client, userPrompt)
		if err != nil {
			return nil, fmt.Errorf("openai retry for %s: %w", reqURL, err)
		}
	}

	return result, nil
}

// callAPI makes a single API call and parses the response JSON.
// Returns nil EvalResult (without error) if the response is not valid JSON.
func (e *OpenAIEvaluator) callAPI(ctx context.Context, client *openai.Client, userPrompt string) (*EvalResult, error) {
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: e.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("openai API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("openai returned no choices")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	return parseEvalResponse(content)
}

// parseEvalResponse parses the JSON response from the model.
// Returns errInvalidJSON if JSON is invalid.
func parseEvalResponse(content string) (*EvalResult, error) {
	var raw struct {
		Good     bool   `json:"good"`
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, errInvalidJSON
	}

	return &EvalResult{Good: raw.Good, Selector: raw.Selector}, nil
}

func buildUserPrompt(reqURL, extractedText, htmlBody, prevSelector string) string {
	if runes := []rune(extractedText); len(runes) > maxExtractedTextLen {
		extractedText = string(runes[:maxExtractedTextLen])
	}
	if runes := []rune(htmlBody); len(runes) > maxHTMLBodyLen {
		htmlBody = string(runes[:maxHTMLBodyLen])
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "I extracted content from this URL: %s\n\n", reqURL)
	_, _ = fmt.Fprintf(&sb, "Extracted text (first 2000 chars):\n---\n%s\n---\n\n", extractedText)
	_, _ = fmt.Fprintf(&sb, "Page HTML structure (first 4000 chars):\n---\n%s\n---\n\n", htmlBody)
	_, _ = fmt.Fprint(&sb, `Is this a good extraction of the article content? Consider:
- Does it contain the main article body (not just navigation/ads/boilerplate)?
- Is it reasonably complete (not truncated or empty)?

Respond in JSON only, no other text:
{"good": true} if extraction is fine
{"good": false, "selector": "article.post-content"} if not, with a CSS selector that targets the main content on this page`)

	if prevSelector != "" {
		_, _ = fmt.Fprintf(&sb, "\n\nPrevious attempt with selector %q was tried but didn't improve. "+
			"Suggest a different selector based on the HTML structure above.", prevSelector)
	}

	return sb.String()
}
