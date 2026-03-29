package extractor

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ukeeper/ukeeper-readability/datastore"
	"github.com/ukeeper/ukeeper-readability/extractor/mocks"
)

func TestExtractURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/IAvTHr" {
			http.Redirect(w, r, "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", http.StatusFound)
			return
		}
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=windows-1251") // test non-standard charset decoding
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
		if r.URL.String() == "/bad_body" {
			w.Header().Set("Content-Length", "1") // error on reading body
			return
		}
	}))
	defer ts.Close()

	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200}

	tests := []struct {
		name           string
		url            string
		wantURL        string
		wantTitle      string
		wantContentLen int
		wantErr        bool
	}{
		{
			name:           "full url",
			url:            ts.URL + "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/",
			wantURL:        ts.URL + "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/",
			wantTitle:      "Всем миром для общей пользы • Umputun тут был",
			wantContentLen: 9665,
			wantErr:        false,
		},
		{
			name:           "short url",
			url:            ts.URL + "/IAvTHr",
			wantURL:        ts.URL + "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/",
			wantTitle:      "Всем миром для общей пользы • Umputun тут был",
			wantContentLen: 9665,
			wantErr:        false,
		},
		{
			name:    "bad body",
			url:     ts.URL + "/bad_body",
			wantErr: true,
		},
		{
			name:    "bad url",
			url:     "http://bad_url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb, err := lr.Extract(context.Background(), tt.url)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, rb)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, rb.URL)
			assert.Equal(t, tt.wantTitle, rb.Title)
			assert.Len(t, rb.Content, tt.wantContentLen)
		})
	}
}

func TestExtractGeneral(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/v48b6Q" {
			http.Redirect(w, r, "/p/2015/11/22/podcast-369/", http.StatusFound)
			return
		}
		if r.URL.String() == "/p/2015/11/22/podcast-369/" {
			fh, err := os.Open("testdata/podcast-369.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	require.NoError(t, err)

	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200}
	a, err := lr.Extract(context.Background(), ts.URL+"/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/")
	require.NoError(t, err)
	assert.Equal(t, "Всем миром для общей пользы • Umputun тут был", a.Title)
	assert.Equal(t, ts.URL+"/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/", a.URL)
	assert.Equal(t, "Не первый раз я практикую идею “а давайте, ребята, сделаем для общего блага …”, и вот опять. В нашем подкасте радио-т есть незаменимый инструмент, позволяющий собирать новости, готовить их к выпуску, ...", a.Excerpt)
	assert.Equal(t, tsURL.Host, a.Domain)

	a, err = lr.Extract(context.Background(), ts.URL+"/v48b6Q")
	require.NoError(t, err)
	assert.Equal(t, "UWP - Выпуск 369", a.Title)
	assert.Equal(t, ts.URL+"/p/2015/11/22/podcast-369/", a.URL)
	assert.Equal(t, "2015-11-22 Нагло ходил в гости. Табличка на двери сработала на 50%Никогда нас школа не хвалила. Девочка осваивает новый прибор. Мое неприятие их логики. И разошлись по будкам …Отбиваюсь от опасных ...", a.Excerpt)
	assert.Equal(t, "https://podcast.umputun.com/images/uwp/uwp369.jpg", a.Image)
	assert.Equal(t, tsURL.Host, a.Domain)
	assert.Len(t, a.AllLinks, 13)
	assert.Contains(t, a.AllLinks, "https://podcast.umputun.com/media/ump_podcast369.mp3")
	assert.Contains(t, a.AllLinks, "https://podcast.umputun.com/images/uwp/uwp369.jpg")
	log.Printf("links=%v", a.AllLinks)
}

func TestNormalizeLinks(t *testing.T) {
	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200}
	inp := `blah <img src="/aaa.png"/> sdfasd <a href="/blah2/aa.link">something</a> blah33 <img src="//aaa.com/xyz.jpg">xx</img>`
	u, _ := url.Parse("http://ukeeper.com/blah")
	out, links := lr.normalizeLinks(inp, u)
	assert.Equal(t, `blah <img src="http://ukeeper.com/aaa.png"/> sdfasd <a href="http://ukeeper.com/blah2/aa.link">something</a> blah33 <img src="http://aaa.com/xyz.jpg">xx</img>`, out)
	assert.Len(t, links, 3)

	inp = `<body>
		<img class="alignright size-full wp-image-944214 lazyloadableImage lazyLoad-fadeIn" alt="View Page Source" width="308" height="508" data-original="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg" src="http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg"></body>`
	_, links = lr.normalizeLinks(inp, u)
	assert.Len(t, links, 1)
	assert.Equal(t, "http://cdn1.tnwcdn.com/wp-content/blogs.dir/1/files/2016/01/page-source.jpg", links[0])
}

func TestNormalizeLinksIssue(t *testing.T) {
	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200}
	_, err := lr.Extract(context.Background(), "https://git-scm.com/book/en/v2/Git-Tools-Submodules")
	require.NoError(t, err)
}

func TestExtractByRule(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/2015/09/25/poiezdka-s-apple-maps/" {
			fh, err := os.Open("testdata/poiezdka-s-apple-maps.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
	}))
	defer ts.Close()

	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200}

	t.Run("with custom rule", func(t *testing.T) {
		rule := &datastore.Rule{Content: ".content p", Enabled: true}
		res, err := lr.ExtractByRule(context.Background(), ts.URL+"/2015/09/25/poiezdka-s-apple-maps/", rule)
		require.NoError(t, err)
		assert.NotEmpty(t, res.Content)
		assert.NotEmpty(t, res.Rich)
		assert.NotEmpty(t, res.Title)
		assert.Contains(t, res.URL, "/2015/09/25/poiezdka-s-apple-maps/")
	})

	t.Run("without rule falls back to general parser", func(t *testing.T) {
		res, err := lr.ExtractByRule(context.Background(), ts.URL+"/2015/09/25/poiezdka-s-apple-maps/", nil)
		require.NoError(t, err)
		assert.NotEmpty(t, res.Content)
		assert.NotEmpty(t, res.Title)
	})

	t.Run("bad url", func(t *testing.T) {
		rule := &datastore.Rule{Content: "article", Enabled: true}
		res, err := lr.ExtractByRule(context.Background(), "http://bad_url", rule)
		require.Error(t, err)
		assert.Nil(t, res)
	})
}

func TestExtractWithCustomRetriever(t *testing.T) {
	testHTML := `<html><head><title>Test Page</title></head>
<body><article><p>This is the article content from a custom retriever.</p></article></body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{
				Body:   []byte(testHTML),
				URL:    reqURL,
				Header: header,
			}, nil
		},
	}

	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200, Retriever: mockRetriever}
	res, err := lr.Extract(context.Background(), "https://example.com/test-page")
	require.NoError(t, err)

	assert.Equal(t, "Test Page", res.Title)
	assert.Equal(t, "https://example.com/test-page", res.URL)
	assert.Equal(t, "example.com", res.Domain)
	assert.NotEmpty(t, res.Content)
	assert.Contains(t, res.Content, "article content from a custom retriever")

	calls := mockRetriever.RetrieveCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "https://example.com/test-page", calls[0].URL)
}

func TestExtractWithEvaluatorGoodOnFirstTry(t *testing.T) {
	testHTML := `<html><head><title>Test Article</title></head>
<body><article><p>This is excellent article content that was extracted properly.</p></article></body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	evalCalls := 0
	mockEvaluator := &AIEvaluatorMock{
		EvaluateFunc: func(_ context.Context, _, _, _, _ string) (*EvalResult, error) {
			evalCalls++
			return &EvalResult{Good: true}, nil
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		AIEvaluator: mockEvaluator,
	}

	res, err := lr.Extract(context.Background(), "https://example.com/article")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Content)
	assert.Contains(t, res.Content, "excellent article content")
	assert.Equal(t, 1, evalCalls, "should call evaluator exactly once")
}

func TestExtractWithEvaluatorBadThenImproved(t *testing.T) {
	testHTML := `<html><head><title>Test Article</title></head>
<body>
<nav>Navigation menu items</nav>
<div class="article-body"><p>This is the real article content that should be extracted.</p></div>
<footer>Footer stuff</footer>
</body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	evalCalls := 0
	mockEvaluator := &AIEvaluatorMock{
		EvaluateFunc: func(_ context.Context, _, _, _, _ string) (*EvalResult, error) {
			evalCalls++
			if evalCalls == 1 {
				return &EvalResult{Good: false, Selector: "div.article-body"}, nil
			}
			return &EvalResult{Good: true}, nil
		},
	}

	saveCalled := false
	mockRules := &mocks.RulesMock{
		GetFunc: func(_ context.Context, _ string) (datastore.Rule, bool) {
			return datastore.Rule{}, false // no existing rule
		},
		SaveFunc: func(_ context.Context, rule datastore.Rule) (datastore.Rule, error) {
			saveCalled = true
			assert.Equal(t, "example.com", rule.Domain)
			assert.Equal(t, "div.article-body", rule.Content)
			assert.True(t, rule.Enabled)
			assert.Equal(t, "ai-evaluator", rule.User)
			return rule, nil
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		AIEvaluator: mockEvaluator,
		Rules:       mockRules,
	}

	res, err := lr.Extract(context.Background(), "https://example.com/article")
	require.NoError(t, err)
	assert.Contains(t, res.Content, "real article content")
	assert.Equal(t, 2, evalCalls, "should call evaluator twice (bad then good)")
	assert.True(t, saveCalled, "should save the rule")
}

func TestExtractWithoutEvaluator(t *testing.T) {
	testHTML := `<html><head><title>Test</title></head>
<body><article><p>Content here.</p></article></body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		// no AIEvaluator set
	}

	res, err := lr.Extract(context.Background(), "https://example.com/test")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Content)
	assert.Equal(t, "Test", res.Title)
}

func TestExtractWithEvaluatorError(t *testing.T) {
	testHTML := `<html><head><title>Test</title></head>
<body><article><p>Original content.</p></article></body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	mockEvaluator := &AIEvaluatorMock{
		EvaluateFunc: func(_ context.Context, _, _, _, _ string) (*EvalResult, error) {
			return nil, fmt.Errorf("openai API error: connection refused")
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		AIEvaluator: mockEvaluator,
	}

	res, err := lr.Extract(context.Background(), "https://example.com/test")
	require.NoError(t, err, "should not fail even when evaluator errors")
	assert.NotEmpty(t, res.Content)
	assert.Contains(t, res.Content, "Original content")
}

func TestExtractSkipsEvaluationWhenRuleExists(t *testing.T) {
	testHTML := `<html><head><title>Test</title></head>
<body><div class="post"><p>Post content.</p></div></body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	evalCalled := false
	mockEvaluator := &AIEvaluatorMock{
		EvaluateFunc: func(_ context.Context, _, _, _, _ string) (*EvalResult, error) {
			evalCalled = true
			return &EvalResult{Good: true}, nil
		},
	}

	mockRules := &mocks.RulesMock{
		GetFunc: func(_ context.Context, _ string) (datastore.Rule, bool) {
			return datastore.Rule{Content: "div.post", Enabled: true}, true // existing rule
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		AIEvaluator: mockEvaluator,
		Rules:       mockRules,
	}

	res, err := lr.Extract(context.Background(), "https://example.com/test")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Content)
	assert.False(t, evalCalled, "should not call evaluator when domain has existing rule")
}

func TestExtractAndImproveForceMode(t *testing.T) {
	testHTML := `<html><head><title>Test Article</title></head>
<body>
<nav>Nav stuff</nav>
<div class="main-content"><p>The real article text that should be found.</p></div>
<aside>Sidebar</aside>
</body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	evalCalls := 0
	mockEvaluator := &AIEvaluatorMock{
		EvaluateFunc: func(_ context.Context, _, _, _, _ string) (*EvalResult, error) {
			evalCalls++
			if evalCalls == 1 {
				return &EvalResult{Good: false, Selector: "div.main-content"}, nil
			}
			return &EvalResult{Good: true}, nil
		},
	}

	// existing rule should be ignored in force mode
	mockRules := &mocks.RulesMock{
		GetFunc: func(_ context.Context, _ string) (datastore.Rule, bool) {
			return datastore.Rule{Content: "nav", Enabled: true}, true
		},
		SaveFunc: func(_ context.Context, rule datastore.Rule) (datastore.Rule, error) {
			assert.Equal(t, "div.main-content", rule.Content)
			return rule, nil
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		AIEvaluator: mockEvaluator,
		Rules:       mockRules,
	}

	res, err := lr.ExtractAndImprove(context.Background(), "https://example.com/article")
	require.NoError(t, err)
	assert.Contains(t, res.Content, "real article text")
	assert.True(t, evalCalls >= 1, "should call evaluator even though rule exists")

	// verify Get was NOT used for extraction (force mode skips stored rules)
	// the Get mock returns "nav" selector which would extract "Nav stuff"
	// but we should have "real article text" from the AI-suggested selector
	assert.NotContains(t, res.Content, "Nav stuff")
}

func TestExtractWithEvaluatorBadSelectorNoMatch(t *testing.T) {
	testHTML := `<html><head><title>Test</title></head>
<body><article><p>Original content from readability.</p></article></body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	evalCalls := 0
	mockEvaluator := &AIEvaluatorMock{
		EvaluateFunc: func(_ context.Context, _, _, _, _ string) (*EvalResult, error) {
			evalCalls++
			if evalCalls <= 2 {
				return &EvalResult{Good: false, Selector: "div.nonexistent"}, nil
			}
			return &EvalResult{Good: true}, nil
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		AIEvaluator: mockEvaluator,
		MaxGPTIter:  3,
	}

	res, err := lr.Extract(context.Background(), "https://example.com/test")
	require.NoError(t, err)
	assert.Contains(t, res.Content, "Original content from readability")
	assert.Equal(t, 3, evalCalls, "should iterate all 3 times")
}

func TestExtractWithEvaluatorSaveFailure(t *testing.T) {
	testHTML := `<html><head><title>Test</title></head>
<body>
<div class="article-body"><p>Real article content here.</p></div>
</body></html>`

	mockRetriever := &RetrieverMock{
		RetrieveFunc: func(_ context.Context, reqURL string) (*RetrieveResult, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/html; charset=utf-8")
			return &RetrieveResult{Body: []byte(testHTML), URL: reqURL, Header: header}, nil
		},
	}

	evalCalls := 0
	mockEvaluator := &AIEvaluatorMock{
		EvaluateFunc: func(_ context.Context, _, _, _, _ string) (*EvalResult, error) {
			evalCalls++
			if evalCalls == 1 {
				return &EvalResult{Good: false, Selector: "div.article-body"}, nil
			}
			return &EvalResult{Good: true}, nil
		},
	}

	mockRules := &mocks.RulesMock{
		GetFunc: func(_ context.Context, _ string) (datastore.Rule, bool) {
			return datastore.Rule{}, false
		},
		SaveFunc: func(_ context.Context, _ datastore.Rule) (datastore.Rule, error) {
			return datastore.Rule{}, fmt.Errorf("mongo connection refused")
		},
	}

	lr := UReadability{
		TimeOut:     30 * time.Second,
		SnippetSize: 200,
		Retriever:   mockRetriever,
		AIEvaluator: mockEvaluator,
		Rules:       mockRules,
	}

	res, err := lr.Extract(context.Background(), "https://example.com/article")
	require.NoError(t, err, "save failure should not propagate")
	assert.Contains(t, res.Content, "Real article content")
}

func TestGetContentCustom(t *testing.T) {
	rulesMock := &mocks.RulesMock{
		GetFunc: func(_ context.Context, _ string) (datastore.Rule, bool) {
			return datastore.Rule{Content: "#content p, .post-title"}, true
		},
	}
	lr := UReadability{TimeOut: 30 * time.Second, SnippetSize: 200, Rules: rulesMock}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/2015/09/25/poiezdka-s-apple-maps/" {
			fh, err := os.Open("testdata/poiezdka-s-apple-maps.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
	}))
	defer ts.Close()
	resp, err := httpClient.Get(ts.URL + "/2015/09/25/poiezdka-s-apple-maps/")
	require.NoError(t, err)
	defer resp.Body.Close()
	dataBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := string(dataBytes)

	content, rich, err := lr.getContent(context.Background(), body, ts.URL+"/2015/09/25/poiezdka-s-apple-maps/", nil)
	require.NoError(t, err)
	assert.Len(t, content, 6988)
	assert.Len(t, rich, 7169)
}

func TestUReadability_GenerateSummary(t *testing.T) {
	mockOpenAI := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(_ context.Context,
			_ openai.ChatCompletionRequest,
		) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: "This is a summary of the article.",
						},
					},
				},
			}, nil
		},
	}

	// cache hit test setup
	cachedContent := "This is a cached content"
	mockSummariesWithCache := &mocks.SummariesMock{
		GetFunc: func(_ context.Context, content string) (datastore.Summary, bool) {
			if content == cachedContent {
				return datastore.Summary{
					ID:        "cached-id",
					Content:   cachedContent,
					Summary:   "This is a cached summary.",
					Model:     "mini",
					CreatedAt: time.Now(),
				}, true
			}
			return datastore.Summary{}, false
		},
	}

	// cache miss and save test setup
	var savedSummary datastore.Summary
	mockSummariesWithSave := &mocks.SummariesMock{
		GetFunc: func(_ context.Context, _ string) (datastore.Summary, bool) {
			return datastore.Summary{}, false
		},
		SaveFunc: func(_ context.Context, summary datastore.Summary) error {
			savedSummary = summary
			return nil
		},
	}

	// cache save error test setup
	mockSummariesWithError := &mocks.SummariesMock{
		GetFunc: func(_ context.Context, _ string) (datastore.Summary, bool) {
			return datastore.Summary{}, false
		},
		SaveFunc: func(_ context.Context, _ datastore.Summary) error {
			return fmt.Errorf("failed to save summary")
		},
	}

	tests := []struct {
		name           string
		content        string
		apiKey         string
		modelType      string
		summaries      Summaries
		expectedResult string
		expectedError  string
		checkSaved     bool
	}{
		{
			name:           "valid api key and content",
			content:        "This is a test article content.",
			apiKey:         "test-key",
			expectedResult: "This is a summary of the article.",
			expectedError:  "",
		},
		{
			name:           "no api key",
			content:        "This is a test article content.",
			apiKey:         "",
			expectedResult: "",
			expectedError:  "API key for summarization is not set",
		},
		{
			name:           "cache hit",
			content:        cachedContent,
			apiKey:         "test-key",
			summaries:      mockSummariesWithCache,
			expectedResult: "This is a cached summary.",
			expectedError:  "",
		},
		{
			name:           "cache miss with save",
			content:        "This is uncached content",
			apiKey:         "test-key",
			modelType:      "non-default",
			summaries:      mockSummariesWithSave,
			expectedResult: "This is a summary of the article.",
			expectedError:  "",
			checkSaved:     true,
		},
		{
			name:           "cache save error",
			content:        "This is uncached content",
			apiKey:         "test-key",
			summaries:      mockSummariesWithError,
			expectedResult: "This is a summary of the article.",
			expectedError:  "",
		},
		{
			name:           "direct model specification",
			content:        "This is a test with direct model name.",
			apiKey:         "test-key",
			modelType:      "gpt-4o",
			expectedResult: "This is a summary of the article.",
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := UReadability{
				OpenAIKey:     tt.apiKey,
				ModelType:     tt.modelType,
				Summaries:     tt.summaries,
				apiClient:     mockOpenAI,
				OpenAIEnabled: true,
			}

			result, err := r.GenerateSummary(context.Background(), tt.content)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			if tt.checkSaved {
				truncateLength := 1000
				expectedContent := tt.content
				if len(tt.content) > truncateLength {
					expectedContent = tt.content[:truncateLength]
				}
				assert.Equal(t, expectedContent, savedSummary.Content)
				assert.Equal(t, "This is a summary of the article.", savedSummary.Summary)

				expectedModel := tt.modelType
				if tt.modelType == "" {
					expectedModel = openai.GPT4oMini
				}
				assert.Equal(t, expectedModel, savedSummary.Model)
			}
		})
	}
}
