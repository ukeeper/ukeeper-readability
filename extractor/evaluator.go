package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	log "github.com/go-pkgz/lgr"
	openai "github.com/sashabaranov/go-openai"
)

//go:generate moq -out evaluator_mock_test.go -skip-ensure -fmt goimports . AIEvaluator

// AIEvaluator evaluates extraction quality and suggests CSS selectors for improvement
type AIEvaluator interface {
	Evaluate(ctx context.Context, url, extractedText, htmlBody string) (*EvalResult, error)
}

// EvalResult holds the evaluation outcome from the AI model
type EvalResult struct {
	Good     bool   // true if extraction looks fine
	Selector string // suggested CSS selector (only when Good=false)
}

const (
	maxExtractedTextLen = 2000
	maxHTMLBodyLen      = 4000
)

const systemPrompt = `You are a web content extraction expert. You evaluate whether extracted article text is complete and correct, and suggest CSS selectors when extraction is poor.`

// OpenAIEvaluator uses OpenAI API to evaluate extraction quality
type OpenAIEvaluator struct {
	APIKey       string
	Model        string
	clientConfig *openai.ClientConfig // optional, for testing
}

// Evaluate sends the extracted text and HTML body to OpenAI for evaluation.
// Returns EvalResult indicating whether extraction is good, or suggests a CSS selector.
func (e *OpenAIEvaluator) Evaluate(ctx context.Context, reqURL, extractedText, htmlBody string) (*EvalResult, error) {
	var client *openai.Client
	if e.clientConfig != nil {
		client = openai.NewClientWithConfig(*e.clientConfig)
	} else {
		client = openai.NewClient(e.APIKey)
	}

	userPrompt := buildUserPrompt(reqURL, extractedText, htmlBody, "")
	result, err := e.callAPI(ctx, client, userPrompt)
	if err != nil {
		return nil, err
	}

	// retry once on invalid JSON
	if result == nil {
		log.Printf("[WARN] invalid JSON from OpenAI for %s, retrying once", reqURL)
		result, err = e.callAPI(ctx, client, userPrompt)
		if err != nil {
			return nil, err
		}
		if result == nil {
			log.Printf("[WARN] invalid JSON from OpenAI for %s on retry, failing open", reqURL)
			return &EvalResult{Good: true}, nil
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
		return nil, fmt.Errorf("openai returned no choices")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	return parseEvalResponse(content)
}

// parseEvalResponse parses the JSON response from the model.
// Returns nil EvalResult (without error) if JSON is invalid.
func parseEvalResponse(content string) (*EvalResult, error) {
	var raw struct {
		Good     bool   `json:"good"`
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, nil //nolint:nilnil // nil result signals invalid JSON, handled by caller
	}

	return &EvalResult{Good: raw.Good, Selector: raw.Selector}, nil
}

func buildUserPrompt(reqURL, extractedText, htmlBody, prevSelector string) string {
	if len(extractedText) > maxExtractedTextLen {
		extractedText = extractedText[:maxExtractedTextLen]
	}
	if len(htmlBody) > maxHTMLBodyLen {
		htmlBody = htmlBody[:maxHTMLBodyLen]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("I extracted content from this URL: %s\n\n", reqURL))
	sb.WriteString("Extracted text (first 2000 chars):\n---\n")
	sb.WriteString(extractedText)
	sb.WriteString("\n---\n\n")
	sb.WriteString("Page HTML structure (first 4000 chars):\n---\n")
	sb.WriteString(htmlBody)
	sb.WriteString("\n---\n\n")
	sb.WriteString(`Is this a good extraction of the article content? Consider:
- Does it contain the main article body (not just navigation/ads/boilerplate)?
- Is it reasonably complete (not truncated or empty)?

Respond in JSON only, no other text:
{"good": true} if extraction is fine
{"good": false, "selector": "article.post-content"} if not, with a CSS selector that targets the main content on this page`)

	if prevSelector != "" {
		sb.WriteString(fmt.Sprintf("\n\nPrevious attempt with selector %q was tried but didn't improve. "+
			"Suggest a different selector based on the HTML structure above.", prevSelector))
	}

	return sb.String()
}
