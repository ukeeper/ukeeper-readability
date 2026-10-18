package extractor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ukeeper/ukeeper-readability/datastore"
	"github.com/ukeeper/ukeeper-readability/extractor/mocks"
)

// articleText is long enough to pass the minimum length check
var articleText = strings.Repeat("The quick brown fox jumps over the lazy dog. ", 12)

// commentsText is the kind of text the general parser prefers over the article: long, comma-rich paragraphs
var commentsText = strings.Repeat("Well, I think so, but then again, who knows, maybe not, and yet here we are, arguing about it, again. ", 6)

// storyPage keeps the article in div.copy, followed by a bigger block of comments that the general
// parser picks instead. div#page and body hold all of it, so both are too wide to be the article.
func storyPage() string {
	comments := ""
	for range 8 {
		comments += "<p>" + commentsText + "</p>"
	}
	return `<html><head><title>Story title</title><style>.x{color:red}</style><script>var tracking = "` +
		strings.Repeat("x", 500) + `";</script></head><body><div id="page">
<ul class="menu"><li>Home</li><li>News</li><li>Contact</li></ul>
<div class="copy"><p>` + articleText + `</p><p>` + articleText + `</p><p>` + articleText + `</p></div>
<div class="thread">` + comments + `</div>
</div></body></html>`
}

func jsonSelector(selector string) string {
	return fmt.Sprintf(`{"selector": %q}`, selector)
}

// sequence answers each call with the next reply
func sequence(replies ...string) func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	var i int
	return func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
		reply := replies[min(i, len(replies)-1)]
		i++
		return answer(reply)(ctx, req)
	}
}

func suggestServer(t *testing.T, page string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(ts.Close)
	return ts
}

// rulesStore is a Rules mock that records SaveIfAbsent calls and refuses to save over a rule of the
// same domain, as the datastore does; existing are the rules the domain already has. Save is not set,
// so a call to it panics: replacing a rule is never allowed here.
func rulesStore(existing ...datastore.Rule) *mocks.RulesMock {
	return &mocks.RulesMock{
		GetFunc: func(_ context.Context, rURL string) (datastore.Rule, bool) {
			u, _ := url.Parse(rURL)
			for _, r := range existing {
				if r.Domain == u.Host && r.Enabled {
					return r, true
				}
			}
			return datastore.Rule{}, false
		},
		SaveIfAbsentFunc: func(_ context.Context, rule datastore.Rule) (bool, error) {
			for _, r := range existing {
				if r.Domain == rule.Domain {
					return false, nil
				}
			}
			return true, nil
		},
	}
}

func newSuggester(client OpenAIClient, rules Rules) *UReadability {
	return &UReadability{TimeOut: 5 * time.Second, SnippetSize: 100, OpenAI: client, Rules: rules}
}

func TestSuggestRule(t *testing.T) {
	t.Run("disabled without client", func(t *testing.T) {
		r := UReadability{}
		_, err := r.SuggestRule(context.Background(), "http://example.com/a")
		assert.ErrorIs(t, err, ErrOpenAIDisabled)
	})

	t.Run("good selector is stored disabled and used for the response", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer(jsonSelector("div.copy"))}
		rules := rulesStore()

		res, err := newSuggester(client, rules).SuggestRule(context.Background(), ts.URL+"/article")
		require.NoError(t, err)
		assert.Equal(t, "div.copy", res.Selector)
		assert.True(t, res.Saved)
		assert.Contains(t, res.Response.Content, "The quick brown fox")
		assert.NotContains(t, res.Response.Content, "Well, I think so")
		assert.NotContains(t, res.Response.Content, "Contact")

		require.Len(t, rules.SaveIfAbsentCalls(), 1)
		saved := rules.SaveIfAbsentCalls()[0].Rule
		host, _ := url.Parse(ts.URL)
		assert.Equal(t, datastore.Rule{Domain: host.Host, Content: "div.copy", TestURLs: []string{ts.URL + "/article"}, User: "openai"}, saved)
		assert.False(t, saved.Enabled, "an unreviewed rule must not affect readers")

		require.Len(t, client.CreateChatCompletionCalls(), 1)
		req := client.CreateChatCompletionCalls()[0].Request
		assert.Equal(t, openai.GPT6Luna, req.Model)
		assert.Equal(t, openai.ChatCompletionResponseFormatTypeJSONObject, req.ResponseFormat.Type)
		assert.Zero(t, req.MaxCompletionTokens, "reasoning models reject token limits")
		assert.Zero(t, req.MaxTokens, "reasoning models reject max_tokens") //nolint:staticcheck // the validator rejects the deprecated field
		assert.Zero(t, req.Temperature, "reasoning models reject temperature")
		require.Len(t, req.Messages, 2)
		prompt := req.Messages[1].Content
		assert.Contains(t, prompt, ts.URL+"/article")
		assert.Contains(t, prompt, "div.copy text=")
		assert.NotContains(t, prompt, "tracking", "script text is not part of the outline")
	})

	t.Run("custom model", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer(jsonSelector("div.copy"))}
		r := newSuggester(client, rulesStore())
		r.OpenAIModel = "other-model"
		_, err := r.SuggestRule(context.Background(), ts.URL+"/article")
		require.NoError(t, err)
		assert.Equal(t, "other-model", client.CreateChatCompletionCalls()[0].Request.Model)
	})

	rejected := []struct {
		name   string
		reply  string
		reason string
	}{
		{"matches nothing", jsonSelector("div.nothing"), `"div.nothing" was rejected: it matches nothing on the page`},
		{"too little text", jsonSelector("ul.menu"), `"ul.menu" was rejected: it matches only`},
		{"almost the whole page", jsonSelector("div#page"), `"div#page" was rejected: it matches almost all the text of the page`},
		{"too long", jsonSelector("div" + strings.Repeat(".copy", 50)), "was rejected: it is too long"},
		{"not json", "div.copy", "an earlier attempt failed: the reply was not the requested JSON"},
		{"empty selector", jsonSelector("  "), "an earlier attempt failed: the reply was not the requested JSON"},
	}
	for _, tt := range rejected {
		t.Run("rejected then retried: "+tt.name, func(t *testing.T) {
			ts := suggestServer(t, storyPage())
			client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: sequence(tt.reply, jsonSelector("div.copy"))}
			rules := rulesStore()

			res, err := newSuggester(client, rules).SuggestRule(context.Background(), ts.URL+"/article")
			require.NoError(t, err)
			assert.Equal(t, "div.copy", res.Selector)

			require.Len(t, client.CreateChatCompletionCalls(), 2)
			assert.Contains(t, strings.ToLower(client.CreateChatCompletionCalls()[1].Request.Messages[1].Content), strings.ToLower(tt.reason))
			require.Len(t, rules.SaveIfAbsentCalls(), 1)
			assert.Equal(t, "div.copy", rules.SaveIfAbsentCalls()[0].Rule.Content, "a rejected selector must never be stored")
		})
	}

	t.Run("same text as the current result is rejected", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		host, _ := url.Parse(ts.URL)
		current := datastore.Rule{Domain: host.Host, Content: "div.copy", Enabled: true}
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: sequence(jsonSelector("div.copy"), jsonSelector("div.copy p:first-child"))}
		rules := rulesStore(current)

		res, err := newSuggester(client, rules).SuggestRule(context.Background(), ts.URL+"/article")
		require.NoError(t, err)
		assert.Equal(t, "div.copy p:first-child", res.Selector)
		assert.Contains(t, client.CreateChatCompletionCalls()[1].Request.Messages[1].Content, "same text as the current result")
		assert.Contains(t, client.CreateChatCompletionCalls()[0].Request.Messages[1].Content, "The text extracted now is reported as wrong")
	})

	t.Run("all attempts rejected", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer(jsonSelector("div.nothing"))}
		rules := rulesStore()

		_, err := newSuggester(client, rules).SuggestRule(context.Background(), ts.URL+"/article")
		assert.ErrorIs(t, err, ErrNoSelector)
		assert.Len(t, client.CreateChatCompletionCalls(), 3)
		assert.Empty(t, rules.SaveIfAbsentCalls())
	})

	t.Run("existing rule is not replaced", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		host, _ := url.Parse(ts.URL)
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
				existing := datastore.Rule{Domain: host.Host, Content: "div.thread", Enabled: enabled}
				client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer(jsonSelector("div.copy"))}
				rules := rulesStore(existing)

				res, err := newSuggester(client, rules).SuggestRule(context.Background(), ts.URL+"/article")
				require.NoError(t, err)
				assert.Equal(t, "div.copy", res.Selector)
				assert.False(t, res.Saved)
				assert.Len(t, rules.SaveIfAbsentCalls(), 1, "the rule is offered to the store, which keeps the existing one")
				assert.Empty(t, rules.SaveCalls(), "Save would replace the rule of the domain")
			})
		}
	})

	t.Run("save failure still returns the suggestion", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer(jsonSelector("div.copy"))}
		rules := rulesStore()
		rules.SaveIfAbsentFunc = func(context.Context, datastore.Rule) (bool, error) { return false, errors.New("db down") }

		res, err := newSuggester(client, rules).SuggestRule(context.Background(), ts.URL+"/article")
		require.NoError(t, err)
		assert.Equal(t, "div.copy", res.Selector)
		assert.False(t, res.Saved)
	})

	t.Run("api error is returned without retry", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{}, errors.New("401 unauthorized")
		}}
		rules := rulesStore()

		_, err := newSuggester(client, rules).SuggestRule(context.Background(), ts.URL+"/article")
		require.ErrorContains(t, err, "401 unauthorized")
		assert.NotErrorIs(t, err, ErrNoSelector)
		assert.Len(t, client.CreateChatCompletionCalls(), 1)
		assert.Empty(t, rules.SaveIfAbsentCalls())
	})

	t.Run("no choices counts as a bad reply", func(t *testing.T) {
		ts := suggestServer(t, storyPage())
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{}, nil
		}}
		_, err := newSuggester(client, rulesStore()).SuggestRule(context.Background(), ts.URL+"/article")
		assert.ErrorIs(t, err, ErrNoSelector)
		assert.Len(t, client.CreateChatCompletionCalls(), 3)
	})

	t.Run("page that can't be fetched never reaches the model", func(t *testing.T) {
		ts := httptest.NewServer(http.NotFoundHandler())
		unreachable := ts.URL
		ts.Close()
		client := &mocks.OpenAIClientMock{CreateChatCompletionFunc: answer(jsonSelector("div.copy"))}
		_, err := newSuggester(client, rulesStore()).SuggestRule(context.Background(), unreachable+"/article")
		require.Error(t, err)
		assert.Empty(t, client.CreateChatCompletionCalls())
	})
}
