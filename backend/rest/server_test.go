package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-pkgz/rest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ukeeper/ukeeper-readability/backend/datastore"
	"github.com/ukeeper/ukeeper-readability/backend/extractor"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyz"

func TestServer_FileServer(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}
	testHTMLName := "test-ureadability.html"
	dir := os.TempDir()
	testHTMLFile := filepath.Join(dir, testHTMLName)
	err := os.WriteFile(testHTMLFile, []byte("some html"), 0o600)
	require.NoError(t, err)

	srv := Server{
		Readability: extractor.UReadability{TimeOut: 30 * time.Second, SnippetSize: 300},
		Credentials: map[string]string{"admin": "password"},
	}
	ts := httptest.NewServer(srv.routes(dir))
	defer ts.Close()

	// no file served because it's outside of static dir
	body, code := get(t, ts.URL+"/"+testHTMLName)
	assert.Equal(t, http.StatusNotFound, code)
	assert.Contains(t, body, "404 page not found")
	ts.Close()

	_ = os.Mkdir(filepath.Join(dir, "static"), 0o700)
	require.NoError(t, os.Rename(testHTMLFile, filepath.Join(dir, "static", testHTMLName)))

	ts = httptest.NewServer(srv.routes(dir))
	body, code = get(t, ts.URL+"/"+testHTMLName)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "some html", body)
	require.NoError(t, os.Remove(filepath.Join(dir, "static", testHTMLName)))
	require.NoError(t, os.Remove(filepath.Join(dir, "static")))
}

func TestServer_Shutdown(t *testing.T) {
	srv := Server{}
	done := make(chan bool)
	ctx, cancel := context.WithCancel(context.Background())

	// without waiting for channel close at the end goroutine will stay alive after test finish
	// which would create data race with next test
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
		close(done)
	}()

	st := time.Now()
	srv.Run(ctx, "127.0.0.1", 0, "../web")
	assert.Less(t, time.Since(st), time.Second, "should take about 100ms")
	<-done
}

func TestServer_WrongAuth(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	// no credentials
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", ts.URL+"/api/rule", strings.NewReader("{}"))
	require.NoError(t, err)
	r, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode)

	// wrong user
	req.SetBasicAuth("wrong_user", "password")
	r, err = client.Do(req)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode)

	// wrong password
	req.SetBasicAuth("admin", "wrong_password")
	r, err = client.Do(req)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode)
}

func TestServer_Extract(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	tss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("../extractor/testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
	}))
	defer tss.Close()

	// happy path
	resp, err := post(t, ts.URL+"/api/extract",
		fmt.Sprintf(`{"url": "%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/"}`, tss.URL))
	require.NoError(t, err)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(b))
	require.NoError(t, resp.Body.Close())
	response := extractor.Response{}
	err = json.Unmarshal(b, &response)
	require.NoError(t, err)

	// legacy endpoint, same response is expected
	legacyBody, code := get(t, ts.URL+"/api/content/v1/parser"+
		fmt.Sprintf(`?url=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/`, tss.URL))
	require.Equal(t, http.StatusOK, code)
	legacyResponse := extractor.Response{}
	err = json.Unmarshal([]byte(legacyBody), &legacyResponse)
	require.NoError(t, err)
	assert.Equal(t, response.Content, legacyResponse.Content)

	// wrong body
	resp, err = post(t, ts.URL+"/api/extract", "wrong_body")
	require.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode, string(b))
	require.NoError(t, resp.Body.Close())

	// no URL
	resp, err = post(t, ts.URL+"/api/extract", "{}")
	require.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(b))
	require.NoError(t, resp.Body.Close())

	// bad URL
	resp, err = post(t, ts.URL+"/api/extract", `{"url": "http://bad_url"}`)
	require.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(b))
	require.NoError(t, resp.Body.Close())
}

func TestServer_LegacyExtract(t *testing.T) {
	ts, srv := startupT(t)
	defer ts.Close()

	tss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("../extractor/testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
	}))
	defer tss.Close()

	// happy path
	b, code := get(t, ts.URL+"/api/content/v1/parser"+
		fmt.Sprintf(`?url=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/`, tss.URL))
	require.Equal(t, http.StatusOK, code)
	resp := extractor.Response{}
	err := json.Unmarshal([]byte(b), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Content)

	// no url
	b, code = get(t, ts.URL+"/api/content/v1/parser")
	require.Equal(t, http.StatusExpectationFailed, code)
	errResponse := rest.JSON{}
	err = json.Unmarshal([]byte(b), &errResponse)
	require.NoError(t, err)
	assert.Equal(t, "no url passed", errResponse["error"])

	// wrong url
	b, code = get(t, ts.URL+"/api/content/v1/parser?url=http://bad_url")
	assert.Equal(t, http.StatusBadRequest, code, b)

	// token
	srv.Token = "secret"
	// no token
	b, code = get(t, ts.URL+"/api/content/v1/parser"+
		fmt.Sprintf(`?url=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/`, tss.URL))
	assert.Equal(t, http.StatusExpectationFailed, code)
	errResponse = rest.JSON{}
	err = json.Unmarshal([]byte(b), &errResponse)
	require.NoError(t, err)
	assert.Equal(t, "no token passed", errResponse["error"])

	// wrong token
	b, code = get(t, ts.URL+"/api/content/v1/parser"+
		fmt.Sprintf(`?url=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/&token=wrong`, tss.URL))
	assert.Equal(t, http.StatusUnauthorized, code)
	errResponse = rest.JSON{}
	err = json.Unmarshal([]byte(b), &errResponse)
	require.NoError(t, err)
	assert.Equal(t, "wrong token passed", errResponse["error"])

	// right token
	b, code = get(t, ts.URL+"/api/content/v1/parser"+
		fmt.Sprintf(`?url=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/&token=secret`, tss.URL))
	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, b)
}

func TestServer_RuleHappyFlow(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()
	randomDomainName := randStringBytesRmndr(42) + ".com"

	// save a rule
	r, err := postFormUrlencoded(t, ts.URL+"/api/rule", fmt.Sprintf(`domain=%s&content=test+content`, randomDomainName))
	require.NoError(t, err)
	rule := datastore.Rule{}
	err = json.NewDecoder(r.Body).Decode(&rule)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, r.StatusCode)
	require.NoError(t, r.Body.Close())
	assert.Equal(t, randomDomainName, rule.Domain)
	assert.Equal(t, "test content", rule.Content)
	ruleID := rule.ID.Hex()

	// get the rule we just saved
	b, code := get(t, ts.URL+"/edit/"+ruleID)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, b, rule.Domain)
	assert.Contains(t, b, rule.Content)

	// check the rule presence in "all" list
	b, code = get(t, ts.URL)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, b, rule.Domain)
	assert.Contains(t, b, rule.Content)

	// disable the rule
	r, err = post(t, ts.URL+"/api/toggle-rule/"+rule.ID.Hex(), "")
	require.NoError(t, err)
	// read body for error message
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode, string(body))
	require.NoError(t, r.Body.Close())

	// get the rule by ID, should look the same as "Enabled" status is only visible on the main page
	b, code = get(t, ts.URL+"/edit/"+ruleID)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, b, rule.Domain)
	assert.Contains(t, b, rule.Content)

	// same disabled rule still should appear in the call to the main page
	b, code = get(t, ts.URL)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, b, rule.Domain)
	assert.Contains(t, b, rule.Content)

	// save the rule with new content, ID should remain the same
	r, err = postFormUrlencoded(t, ts.URL+"/api/rule", fmt.Sprintf(`domain=%s&content=new+content`, randomDomainName))
	require.NoError(t, err)
	rule = datastore.Rule{}
	err = json.NewDecoder(r.Body).Decode(&rule)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, r.StatusCode)
	require.NoError(t, r.Body.Close())
	assert.Equal(t, randomDomainName, rule.Domain)
	assert.Equal(t, "new content", rule.Content)
	assert.Equal(t, ruleID, rule.ID.Hex())
}

func TestServer_RuleUnhappyFlow(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	// save without domain
	r, err := postFormUrlencoded(t, ts.URL+"/api/rule", "")
	require.NoError(t, err)
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	require.Equal(t, http.StatusBadRequest, r.StatusCode)
	assert.Equal(t, "Domain is required\n", string(body))

	// get supposed to fail
	_, code := get(t, ts.URL+"/api/rule")
	assert.Equal(t, http.StatusNotFound, code)

	// get rule by non-existent ID
	_, code = get(t, ts.URL+"/api/rule/nonexistent")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestServer_FakeAuth(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	r, err := post(t, ts.URL+"/api/auth", `""`)
	require.NoError(t, err)
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusOK, r.StatusCode)
	assert.Contains(t, string(body), `"pong":`)
}

func TestServer_HandleIndex(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()
	randomDomainName := randStringBytesRmndr(42) + ".com"

	// add a test rule
	r, err := postFormUrlencoded(t, ts.URL+"/api/rule", fmt.Sprintf(`domain=%s&content=test+content`, randomDomainName))
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())

	// test index page
	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), randomDomainName)
	assert.Contains(t, string(body), "test content")
	assert.Contains(t, string(body), "Правила")
}

func TestServer_HandleAdd(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	// test add page
	resp, err := http.Get(ts.URL + "/add/")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "Добавление правила")
	assert.Contains(t, string(body), `<form hx-post="/api/rule" hx-swap="none">`)
}

func TestServer_HandleEdit(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	randomDomainName := randStringBytesRmndr(42) + ".com"

	// add a test rule
	r, err := postFormUrlencoded(t, ts.URL+"/api/rule", fmt.Sprintf(`domain=%s&content=test+content`, randomDomainName))
	require.NoError(t, err)
	defer r.Body.Close()
	var rule datastore.Rule
	err = json.NewDecoder(r.Body).Decode(&rule)
	require.NoError(t, err)

	// test edit page
	resp, err := http.Get(ts.URL + "/edit/" + rule.ID.Hex())
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "Редактирование правила")
	assert.Contains(t, string(body), randomDomainName)
	assert.Contains(t, string(body), "test content")

	// test edit page for non-existing rule
	resp, err = http.Get(ts.URL + "/edit/non-existing")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Contains(t, string(body), "Rule not found")
}

func TestServer_ToggleRule(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()
	randomDomainName := randStringBytesRmndr(42) + ".com"

	// add a test rule
	r, err := postFormUrlencoded(t, ts.URL+"/api/rule", fmt.Sprintf(`domain=%s&content=test+content`, randomDomainName))
	require.NoError(t, err)
	defer r.Body.Close()
	var rule datastore.Rule
	err = json.NewDecoder(r.Body).Decode(&rule)
	require.NoError(t, err)
	assert.Equal(t, randomDomainName, rule.Domain)
	assert.Equal(t, "test content", rule.Content)
	assert.True(t, rule.Enabled)

	// toggle rule (disable)
	r, err = post(t, ts.URL+"/api/toggle-rule/"+rule.ID.Hex(), "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)

	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	assert.Contains(t, string(body), `class="rules__row rules__row_disabled"`, string(body))

	// toggle rule again (enable)
	r, err = post(t, ts.URL+"/api/toggle-rule/"+rule.ID.Hex(), "")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, r.StatusCode)

	body, err = io.ReadAll(r.Body)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	assert.NotContains(t, string(body), `class="rules__row rules__row_disabled"`)
}

func TestServer_ToggleRuleNotFound(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	// toggle rule (disable)
	r, err := post(t, ts.URL+"/api/toggle-rule/non-existing", "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, r.StatusCode)

	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	assert.Contains(t, string(body), `Rule not found`, string(body))
}

func TestServer_SaveRule(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	randomDomainName := randStringBytesRmndr(42) + ".com"

	// test saving a new rule
	resp, err := postFormUrlencoded(t, ts.URL+"/api/rule", fmt.Sprintf(`domain=%s&content=test+content&author=test+author`, randomDomainName))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("HX-Redirect"))

	// test updating an existing rule
	var rule datastore.Rule
	err = json.NewDecoder(resp.Body).Decode(&rule)
	require.NoError(t, err)

	updatedRule := fmt.Sprintf(`id=%s&domain=%s&content=updated+content&author=updated+author`, rule.ID.Hex(), randomDomainName)
	resp, err = postFormUrlencoded(t, ts.URL+"/api/rule", updatedRule)

	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("HX-Redirect"))

	// verify the rule was updated
	b, code := get(t, ts.URL)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, b, "updated content")

	b, code = get(t, ts.URL+"/edit/"+rule.ID.Hex())
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, b, rule.Domain)
	assert.NotContains(t, b, rule.Content)
	assert.Contains(t, b, "updated content")
	assert.Contains(t, b, "updated author")

	// try to save a rule with the new domain, supposed to fail as ID should be different and can't be altered
	updatedRule = fmt.Sprintf(`id=%s&domain=another_domain&content=updated+content&author=updated+author`, rule.ID.Hex())
	resp, err = postFormUrlencoded(t, ts.URL+"/api/rule", updatedRule)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "duplicate")

	// 10Mb body supposed to hit the form parsing limit
	resp, err = postFormUrlencoded(t, ts.URL+"/api/rule", "domain="+strings.Repeat("a", 10*1024*1024))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Failed to parse form")
}

func TestServer_Preview(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	tss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("../extractor/testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			testHTML, err := io.ReadAll(fh)
			assert.NoError(t, err)
			assert.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			assert.NoError(t, err)
			return
		}
	}))
	defer tss.Close()

	// happy path with no rule
	resp, err := postFormUrlencoded(t, ts.URL+"/api/preview",
		fmt.Sprintf(`test_urls=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/&content=`, tss.URL))
	require.NoError(t, err)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(b))
	require.NoError(t, resp.Body.Close())
	assert.Contains(t, string(b), "<summary>Всем миром для общей пользы • Umputun тут был</summary>")

	// happy path with custom rule
	resp, err = postFormUrlencoded(t, ts.URL+"/api/preview",
		fmt.Sprintf(`test_urls=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/&content=article`, tss.URL))
	require.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(b))
	require.NoError(t, resp.Body.Close())
	assert.Contains(t, string(b), "Всем миром для общей пользы")

	// no URL
	resp, err = post(t, ts.URL+"/api/preview", "")
	require.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(b))
	require.NoError(t, resp.Body.Close())
	assert.Contains(t, string(b), "No preview results available.")

	// 10Mb body supposed to hit the form parsing limit
	resp, err = postFormUrlencoded(t, ts.URL+"/api/preview", "domain="+strings.Repeat("a", 10*1024*1024))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	b, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(b), "Failed to parse form")
}

func TestServer_ExtractArticleEmulateReadabilityWithSummaryFailures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body><p>This is a test article.</p></body></html>"))
	}))
	defer ts.Close()

	tests := []struct {
		name           string
		serverToken    string
		url            string
		token          string
		summary        bool
		expectedStatus int
		expectedError  string
		openAIKey      string
	}{
		{
			name:           "Valid token and summary, no OpenAI key",
			serverToken:    "secret",
			url:            ts.URL,
			token:          "secret",
			summary:        true,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "OpenAI key is not set",
		},
		{
			name:           "No token, summary requested",
			serverToken:    "secret",
			url:            ts.URL,
			summary:        true,
			expectedStatus: http.StatusExpectationFailed,
			expectedError:  "no token passed",
		},
		{
			name:           "Invalid token, summary requested",
			serverToken:    "secret",
			url:            ts.URL,
			token:          "wrong",
			summary:        true,
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "wrong token passed",
			openAIKey:      "test key",
		},
		{
			name:           "Valid token, no summary",
			serverToken:    "secret",
			url:            ts.URL,
			token:          "secret",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "No token, no summary",
			serverToken:    "secret",
			url:            ts.URL,
			expectedStatus: http.StatusExpectationFailed,
		},
		{
			name:           "Server token not set, summary requested",
			serverToken:    "",
			url:            ts.URL,
			token:          "any",
			summary:        true,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "summary generation requires token, but token is not set for the server",
			openAIKey:      "test key",
		},
		{
			name:           "Server token not set, no summary",
			serverToken:    "",
			url:            ts.URL,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := Server{
				Readability: extractor.UReadability{
					TimeOut:     30,
					SnippetSize: 300,
					Rules:       nil,
					OpenAIKey:   tt.openAIKey,
				},
				Token: tt.serverToken,
			}

			url := fmt.Sprintf("/api/content/v1/parser?url=%s", tt.url)
			if tt.token != "" {
				url += fmt.Sprintf("&token=%s", tt.token)
			}
			if tt.summary {
				url += "&summary=true"
			}

			req, err := http.NewRequest("GET", url, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			srv.extractArticleEmulateReadability(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code, rr.Body.String())

			if tt.expectedError != "" {
				var errorResponse map[string]string
				err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedError, errorResponse["error"])
			} else if tt.summary {
				var response extractor.Response
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.Content)
				assert.Equal(t, "This is a summary of the article.", response.Summary)
			}
		})
	}
}

func get(t *testing.T, url string) (response string, statusCode int) {
	r, err := http.Get(url)
	require.NoError(t, err)
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	return string(body), r.StatusCode
}

func post(t *testing.T, url, body string) (*http.Response, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	return client.Do(req)
}

func postFormUrlencoded(t *testing.T, url, body string) (*http.Response, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("admin", "password")
	return client.Do(req)
}

// startupT runs fully configured testing server
func startupT(t *testing.T) (*httptest.Server, *Server) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}

	db, err := datastore.New("mongodb://localhost:27017/", "test_ureadability", 0)
	require.NoError(t, err)

	stores := db.GetStores()
	srv := Server{
		Readability: extractor.UReadability{
			TimeOut:     30 * time.Second,
			SnippetSize: 300,
			Rules:       stores.Rules,
		},
		Credentials: map[string]string{"admin": "password"},
		Version:     "dev-test",
	}

	webDir := "../web"
	templates := template.Must(template.ParseGlob(filepath.Join(webDir, "components", "*.gohtml")))
	srv.indexPage = template.Must(template.Must(templates.Clone()).ParseFiles(filepath.Join(webDir, "index.gohtml")))
	srv.rulePage = template.Must(template.Must(templates.Clone()).ParseFiles(filepath.Join(webDir, "rule.gohtml")))

	return httptest.NewServer(srv.routes(webDir)), &srv
}

// thanks to https://stackoverflow.com/a/31832326/961092
func randStringBytesRmndr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int64()%int64(len(letterBytes))]
	}
	return string(b)
}
