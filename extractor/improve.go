package extractor

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/go-pkgz/lgr"
	"github.com/sashabaranov/go-openai"

	"github.com/ukeeper/ukeeper-readability/datastore"
)

// ErrOpenAIDisabled is returned by SuggestRule when no OpenAI client is configured
var ErrOpenAIDisabled = errors.New("OpenAI is not configured")

// ErrNoSelector is returned by SuggestRule when no attempt produced a selector that passed the checks
var ErrNoSelector = errors.New("no usable selector was suggested")

// errInvalidAnswer marks a model reply that is not the requested JSON
var errInvalidAnswer = errors.New("reply is not the requested JSON")

const (
	suggestAttempts     = 3
	suggestCallTimeout  = 60 * time.Second // one model call
	suggestedRuleUser   = "openai"
	maxSelectorLen      = 200
	minSuggestedLen     = 200 // characters of text a selector has to match
	maxWholePagePercent = 90  // a selector matching more of the page text than this is the page, not the article
	currentTextLen      = 500 // characters of the current extraction shown to the model
)

const suggestPrompt = `You find the main article on a web page. You get an outline of the page body: one line per element ` +
	`with its tag, id, classes, the number of characters of visible text and the number of paragraphs. ` +
	`Reply with JSON only: {"selector": "<CSS selector>"}. ` +
	`The selector must match the element that holds the whole article text and nothing else: ` +
	`no navigation, comments, related links, share buttons or footer. ` +
	`Prefer an id or a stable class over positional selectors.`

// Suggestion is what SuggestRule found for a URL
type Suggestion struct {
	Selector string    `json:"selector"`
	Saved    bool      `json:"saved"`    // true when the selector was stored as a disabled rule for review
	Response *Response `json:"response"` // the page extracted with the selector
}

// attempt is a rejected selector together with the reason, passed back to the model
type attempt struct {
	selector string
	reason   string
}

// SuggestRule asks the model for a CSS selector of the article on the page, for a page the general
// parser got wrong. The selector is applied to the page and has to match a plausible amount of text
// before it is accepted. When the domain has no rule yet, it is stored as a disabled rule, so
// nothing changes for readers until someone reviews and enables it. When the domain already has a
// rule, the existing one is left alone and the suggestion is only returned.
func (f *UReadability) SuggestRule(ctx context.Context, reqURL string) (*Suggestion, error) {
	if f.OpenAI == nil {
		return nil, ErrOpenAIDisabled
	}
	u, err := url.Parse(reqURL)
	if err != nil {
		return nil, fmt.Errorf("parse url %q: %w", reqURL, err)
	}

	var rule *datastore.Rule // the enabled rule, as extraction would use it
	if f.Rules != nil {
		if r, found := f.Rules.Get(ctx, reqURL); found {
			rule = &r
		}
	}

	rb, body, err := f.fetch(ctx, reqURL, rule)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	current, _, _ := f.getContent(ctx, body, reqURL, rule)
	current = f.getText(current, "")
	outline := pageOutline(doc.Nodes[0])
	pageLen := visibleTextLen(findBody(doc.Nodes[0]))

	var rejected []attempt
	for range suggestAttempts {
		selector, err := f.askSelector(ctx, reqURL, current, outline, rejected)
		if errors.Is(err, errInvalidAnswer) {
			rejected = append(rejected, attempt{reason: "the reply was not the requested JSON"})
			continue
		}
		if err != nil {
			return nil, err
		}
		if reason := f.checkSelector(body, selector, current, pageLen); reason != "" {
			log.Printf("[INFO] selector %q for %s rejected: %s", selector, reqURL, reason)
			rejected = append(rejected, attempt{selector: selector, reason: reason})
			continue
		}

		resp, err := f.parse(ctx, reqURL, body, rb, &datastore.Rule{Content: selector, Enabled: true})
		if err != nil {
			return nil, err
		}
		return &Suggestion{Selector: selector, Saved: f.saveSuggested(ctx, u.Host, reqURL, selector), Response: resp}, nil
	}
	return nil, ErrNoSelector
}

// askSelector makes one model call and returns the selector from the reply
func (f *UReadability) askSelector(ctx context.Context, reqURL, current, outline string, rejected []attempt) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "URL: %s\n\n", reqURL)
	if current != "" {
		if r := []rune(current); len(r) > currentTextLen {
			current = string(r[:currentTextLen])
		}
		fmt.Fprintf(&sb, "The text extracted now is reported as wrong. It starts with:\n---\n%s\n---\n\n", current)
	}
	fmt.Fprintf(&sb, "Page body outline:\n---\n%s\n---\n", outline)
	for _, a := range rejected {
		if a.selector == "" {
			fmt.Fprintf(&sb, "\nAn earlier attempt failed: %s. Try again.", a.reason)
			continue
		}
		fmt.Fprintf(&sb, "\nThe selector %q was rejected: %s. Suggest a different one.", a.selector, a.reason)
	}

	callCtx, cancel := context.WithTimeout(ctx, suggestCallTimeout)
	defer cancel()
	resp, err := f.OpenAI.CreateChatCompletion(callCtx, openai.ChatCompletionRequest{
		Model: cmp.Or(f.OpenAIModel, DefaultOpenAIModel),
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: suggestPrompt},
			{Role: openai.ChatMessageRoleUser, Content: sb.String()},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
	})
	if err != nil {
		return "", fmt.Errorf("openai selector request: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices: %w", errInvalidAnswer)
	}

	var answer struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &answer); err != nil || strings.TrimSpace(answer.Selector) == "" {
		return "", errInvalidAnswer
	}
	return strings.TrimSpace(answer.Selector), nil
}

// checkSelector applies the selector to the page and returns why it can't be the article, or "" when it can
func (f *UReadability) checkSelector(body, selector, current string, pageLen int) string {
	if len(selector) > maxSelectorLen {
		return "it is too long"
	}
	res, err := extractWithSelector(body, selector)
	if err != nil || res == "" {
		return "it matches nothing on the page"
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res))
	if err != nil {
		return "what it matches can't be parsed"
	}
	n := visibleTextLen(findBody(doc.Nodes[0]))
	switch {
	case n < minSuggestedLen:
		return fmt.Sprintf("it matches only %d characters of text", n)
	case n*100 > pageLen*maxWholePagePercent:
		return "it matches almost all the text of the page"
	case f.getText(res, "") == current:
		return "it extracts the same text as the current result"
	}
	return ""
}

// saveSuggested stores the selector as a disabled rule when the domain has none. Both enabled and
// disabled rules count, and an existing rule is never replaced.
func (f *UReadability) saveSuggested(ctx context.Context, domain, reqURL, selector string) bool {
	if f.Rules == nil {
		return false
	}
	rule := datastore.Rule{Domain: domain, Content: selector, TestURLs: []string{reqURL}, User: suggestedRuleUser}
	saved, err := f.Rules.SaveIfAbsent(ctx, rule)
	switch {
	case err != nil:
		log.Printf("[WARN] failed to save suggested rule for %s: %v", domain, err)
	case !saved:
		log.Printf("[INFO] %s already has a rule, suggested selector %q is not saved", domain, selector)
	default:
		log.Printf("[INFO] saved disabled rule for %s with selector %q", domain, selector)
	}
	return err == nil && saved
}
