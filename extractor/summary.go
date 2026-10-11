package extractor

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/go-pkgz/lgr"
	"github.com/sashabaranov/go-openai"

	"github.com/ukeeper/ukeeper-readability/datastore"
)

//go:generate moq -out mocks/openai_client.go -pkg mocks -skip-ensure -fmt goimports . OpenAIClient
//go:generate moq -out mocks/summaries.go -pkg mocks -skip-ensure -fmt goimports . Summaries

// OpenAIClient is the part of the OpenAI client used for summaries
type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// Summaries is a cache of generated summaries
type Summaries interface {
	Get(ctx context.Context, id string) (datastore.Summary, bool)
	Save(ctx context.Context, summary datastore.Summary) error
}

// DefaultSummaryPrompt is the default prompt for generating article summaries
const DefaultSummaryPrompt = "You are a helpful assistant that summarizes articles. " +
	"Please summarize the main points in a few sentences as TLDR style (don't add a TLDR label). " +
	"Then, list up to five detailed bullet points. Provide the response in plain text. " +
	"Do not add any additional information. Do not add a Summary at the beginning of the response. " +
	"If detailed bullet points are too similar to the summary, don't include them at all:"

// DefaultOpenAIModel is used when UReadability.OpenAIModel is empty
const DefaultOpenAIModel = openai.GPT6Luna

const (
	summaryTTL         = 30 * 24 * time.Hour
	summaryCallTimeout = 60 * time.Second
)

// ErrSummariesDisabled is returned by GenerateSummary when no OpenAI client is configured
var ErrSummariesDisabled = errors.New("summaries are not configured")

// GenerateSummary returns a summary of the article text, from the cache when available
func (f *UReadability) GenerateSummary(ctx context.Context, content string) (string, error) {
	if f.OpenAI == nil {
		return "", ErrSummariesDisabled
	}
	model := cmp.Or(f.OpenAIModel, DefaultOpenAIModel)
	prompt := cmp.Or(f.SummaryPrompt, DefaultSummaryPrompt)
	if runes := []rune(content); f.MaxContentLength > 0 && len(runes) > f.MaxContentLength {
		content = string(runes[:f.MaxContentLength])
	}

	id := summaryID(model, prompt, content)
	if f.Summaries != nil {
		if cached, ok := f.Summaries.Get(ctx, id); ok {
			return cached.Summary, nil
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, summaryCallTimeout)
	defer cancel()
	resp, err := f.OpenAI.CreateChatCompletion(callCtx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: prompt},
			{Role: openai.ChatMessageRoleUser, Content: content},
		},
	})
	if err != nil {
		return "", fmt.Errorf("openai summary request: %w", err)
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", errors.New("openai returned an empty summary")
	}
	summary := strings.TrimSpace(resp.Choices[0].Message.Content)

	if f.Summaries != nil {
		now := time.Now()
		s := datastore.Summary{ID: id, Summary: summary, Model: model, CreatedAt: now, ExpiresAt: now.Add(summaryTTL)}
		if err := f.Summaries.Save(ctx, s); err != nil {
			log.Printf("[WARN] failed to cache summary, error=%v", err)
		}
	}
	return summary, nil
}

// summaryID keys the cache on everything that shapes the summary, so a new model or prompt misses
func summaryID(model, prompt, content string) string {
	h := sha256.New()
	for _, s := range []string{model, prompt, content} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
