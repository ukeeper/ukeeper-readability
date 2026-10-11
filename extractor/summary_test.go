package extractor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ukeeper/ukeeper-readability/datastore"
	"github.com/ukeeper/ukeeper-readability/extractor/mocks"
)

func answer(text string) func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
		return openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: text}}}}, nil
	}
}

func emptyCache() *mocks.SummariesMock {
	return &mocks.SummariesMock{
		GetFunc:  func(context.Context, string) (datastore.Summary, bool) { return datastore.Summary{}, false },
		SaveFunc: func(context.Context, datastore.Summary) error { return nil },
	}
}

func TestGenerateSummary(t *testing.T) {
	t.Run("disabled without client", func(t *testing.T) {
		r := UReadability{}
		_, err := r.GenerateSummary(context.Background(), "text")
		assert.ErrorIs(t, err, ErrSummariesDisabled)
	})

	t.Run("cache miss calls model and saves", func(t *testing.T) {
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer("  short version\n")}
		cache := emptyCache()
		r := UReadability{OpenAI: client, Summaries: cache}

		res, err := r.GenerateSummary(context.Background(), "article text")
		require.NoError(t, err)
		assert.Equal(t, "short version", res)

		require.Len(t, client.CreateChatCompletionCalls(), 1)
		req := client.CreateChatCompletionCalls()[0].Request
		assert.Equal(t, openai.GPT6Luna, req.Model)
		require.Len(t, req.Messages, 2)
		assert.Equal(t, DefaultSummaryPrompt, req.Messages[0].Content)
		assert.Equal(t, "article text", req.Messages[1].Content)

		require.Len(t, cache.SaveCalls(), 1)
		saved := cache.SaveCalls()[0].Summary
		assert.Equal(t, cache.GetCalls()[0].ID, saved.ID)
		assert.Equal(t, "short version", saved.Summary)
		assert.Equal(t, openai.GPT6Luna, saved.Model)
		assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), saved.ExpiresAt, time.Minute)
	})

	t.Run("cache hit skips model", func(t *testing.T) {
		client := &mocks.OpenAIClientMock{}
		cache := &mocks.SummariesMock{GetFunc: func(context.Context, string) (datastore.Summary, bool) {
			return datastore.Summary{Summary: "cached"}, true
		}}
		r := UReadability{OpenAI: client, Summaries: cache}

		res, err := r.GenerateSummary(context.Background(), "article text")
		require.NoError(t, err)
		assert.Equal(t, "cached", res)
		assert.Empty(t, client.CreateChatCompletionCalls())
	})

	t.Run("custom model and prompt", func(t *testing.T) {
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer("ok")}
		r := UReadability{OpenAI: client, OpenAIModel: "gpt-6.1-sol", SummaryPrompt: "be brief"}

		_, err := r.GenerateSummary(context.Background(), "article text")
		require.NoError(t, err)
		req := client.CreateChatCompletionCalls()[0].Request
		assert.Equal(t, "gpt-6.1-sol", req.Model)
		assert.Equal(t, "be brief", req.Messages[0].Content)
	})

	t.Run("truncates by characters", func(t *testing.T) {
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer("ok")}
		r := UReadability{OpenAI: client, MaxContentLength: 5}

		_, err := r.GenerateSummary(context.Background(), "привет мир")
		require.NoError(t, err)
		assert.Equal(t, "приве", client.CreateChatCompletionCalls()[0].Request.Messages[1].Content)
	})

	t.Run("empty answer is an error", func(t *testing.T) {
		for name, resp := range map[string]openai.ChatCompletionResponse{
			"no choices":  {},
			"blank reply": {Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: " \n"}}}},
		} {
			t.Run(name, func(t *testing.T) {
				cache := emptyCache()
				client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
					return resp, nil
				}}
				r := UReadability{OpenAI: client, Summaries: cache}
				_, err := r.GenerateSummary(context.Background(), "article text")
				require.EqualError(t, err, "openai returned an empty summary")
				assert.Empty(t, cache.SaveCalls())
			})
		}
	})

	t.Run("api error", func(t *testing.T) {
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{}, errors.New("boom")
		}}
		r := UReadability{OpenAI: client}
		_, err := r.GenerateSummary(context.Background(), "article text")
		require.ErrorContains(t, err, "boom")
	})

	t.Run("cache save failure still returns summary", func(t *testing.T) {
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer("ok")}
		cache := emptyCache()
		cache.SaveFunc = func(context.Context, datastore.Summary) error { return errors.New("mongo down") }
		r := UReadability{OpenAI: client, Summaries: cache}

		res, err := r.GenerateSummary(context.Background(), "article text")
		require.NoError(t, err)
		assert.Equal(t, "ok", res)
	})
}

func TestSummaryID(t *testing.T) {
	base := summaryID("m", "p", "c")
	assert.Len(t, base, 64)
	assert.Equal(t, base, summaryID("m", "p", "c"))
	assert.NotEqual(t, base, summaryID("m2", "p", "c"), "model change must miss the cache")
	assert.NotEqual(t, base, summaryID("m", "p2", "c"), "prompt change must miss the cache")
	assert.NotEqual(t, base, summaryID("m", "p", "c2"))
	assert.NotEqual(t, summaryID("ab", "c", "d"), summaryID("a", "bc", "d"), "fields must not run together")
}
