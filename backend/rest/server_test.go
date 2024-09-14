package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-pkgz/rest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ukeeper/ukeeper-redabilty/backend/datastore"
	"github.com/ukeeper/ukeeper-redabilty/backend/extractor"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyz"

func TestServer_FileServer(t *testing.T) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}
	testHTMLName := "test-ureadability.html"
	dir := os.TempDir()
	testHTMLFile := dir + "/" + testHTMLName
	err := os.WriteFile(testHTMLFile, []byte("some html"), 0o700)
	require.NoError(t, err)

	srv := Server{
		Readability: extractor.UReadability{TimeOut: 30, SnippetSize: 300},
		Credentials: map[string]string{"admin": "password"},
	}
	ts := httptest.NewServer(srv.routes(dir))
	defer ts.Close()

	body, code := get(t, ts.URL+"/"+testHTMLName)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "some html", body)
	_ = os.Remove(testHTMLFile)
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
	srv.Run(ctx, "127.0.0.1", 0, ".")
	assert.True(t, time.Since(st).Seconds() < 1, "should take about 100ms")
	<-done
}

func TestServer_WrongAuth(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	// no credentials
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", ts.URL+"/api/rule", strings.NewReader("{}"))
	assert.NoError(t, err)
	r, err := client.Do(req)
	require.NoError(t, err)
	assert.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode)

	// wrong user
	req.SetBasicAuth("wrong_user", "password")
	r, err = client.Do(req)
	require.NoError(t, err)
	assert.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode)

	// wrong password
	req.SetBasicAuth("admin", "wrong_password")
	r, err = client.Do(req)
	require.NoError(t, err)
	assert.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, r.StatusCode)
}

func TestServer_Extract(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	tss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("../extractor/testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			testHTML, err := io.ReadAll(fh)
			require.NoError(t, err)
			require.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			require.NoError(t, err)
			return
		}
	}))
	defer tss.Close()

	// happy path
	resp, err := post(t, ts.URL+"/api/extract",
		fmt.Sprintf(`{"url": "%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/"}`, tss.URL))
	assert.NoError(t, err)
	b, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(b))
	assert.NoError(t, resp.Body.Close())
	response := extractor.Response{}
	err = json.Unmarshal(b, &response)
	assert.NoError(t, err)

	// legacy endpoint, same response is expected
	legacyBody, code := get(t, ts.URL+"/api/content/v1/parser"+
		fmt.Sprintf(`?url=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/`, tss.URL))
	require.Equal(t, http.StatusOK, code)
	legacyResponse := extractor.Response{}
	err = json.Unmarshal([]byte(legacyBody), &legacyResponse)
	assert.NoError(t, err)
	assert.Equal(t, response.Content, legacyResponse.Content)

	// wrong body
	resp, err = post(t, ts.URL+"/api/extract", "wrong_body")
	assert.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode, string(b))
	assert.NoError(t, resp.Body.Close())

	// no URL
	resp, err = post(t, ts.URL+"/api/extract", "{}")
	assert.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(b))
	assert.NoError(t, resp.Body.Close())

	// bad URL
	resp, err = post(t, ts.URL+"/api/extract", `{"url": "http://bad_url"}`)
	assert.NoError(t, err)
	b, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(b))
	assert.NoError(t, resp.Body.Close())
}

func TestServer_LegacyExtract(t *testing.T) {
	ts, srv := startupT(t)
	defer ts.Close()

	tss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == "/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/" {
			fh, err := os.Open("../extractor/testdata/vsiem-mirom-dlia-obshchiei-polzy.html")
			testHTML, err := io.ReadAll(fh)
			require.NoError(t, err)
			require.NoError(t, fh.Close())
			_, err = w.Write(testHTML)
			require.NoError(t, err)
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
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Content)

	// no url
	b, code = get(t, ts.URL+"/api/content/v1/parser")
	require.Equal(t, http.StatusExpectationFailed, code)
	errResponse := rest.JSON{}
	err = json.Unmarshal([]byte(b), &errResponse)
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, "no token passed", errResponse["error"])

	// wrong token
	b, code = get(t, ts.URL+"/api/content/v1/parser"+
		fmt.Sprintf(`?url=%s/2015/11/26/vsiem-mirom-dlia-obshchiei-polzy/&token=wrong`, tss.URL))
	assert.Equal(t, http.StatusUnauthorized, code)
	errResponse = rest.JSON{}
	err = json.Unmarshal([]byte(b), &errResponse)
	assert.NoError(t, err)
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
	r, err := post(t, ts.URL+"/api/rule", fmt.Sprintf(`{"domain": "%s", "content": "test content"}`, randomDomainName))
	require.NoError(t, err)
	rule := datastore.Rule{}
	err = json.NewDecoder(r.Body).Decode(&rule)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, r.StatusCode)
	assert.NoError(t, r.Body.Close())
	assert.Equal(t, randomDomainName, rule.Domain)
	assert.Equal(t, "test content", rule.Content)
	ruleID := rule.ID.Hex()

	// get the rule we just saved
	b, code := get(t, ts.URL+"/api/rule?url=https://"+randomDomainName)
	assert.Equal(t, http.StatusOK, code)
	rule = datastore.Rule{}
	err = json.Unmarshal([]byte(b), &rule)
	require.NoError(t, err)
	assert.Equal(t, randomDomainName, rule.Domain)
	assert.Equal(t, "test content", rule.Content)
	assert.True(t, rule.Enabled)
	assert.Equal(t, ruleID, rule.ID.Hex())

	// check the rule presence in "all" list
	b, code = get(t, ts.URL+"/api/rules")
	assert.Equal(t, http.StatusOK, code)
	var rules []datastore.Rule
	err = json.Unmarshal([]byte(b), &rules)
	require.NoError(t, err)
	assert.Contains(t, rules, rule)

	// get the rule by ID (available after Get call)
	b, code = get(t, ts.URL+"/api/rule/"+fmt.Sprintf(`%s`, ruleID))
	assert.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, b)

	// disable the rule
	r, err = del(t, ts.URL+"/api/rule/"+fmt.Sprintf(`%s`, rule.ID.Hex()))
	assert.NoError(t, err)
	// read body for error message
	body, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode, string(body))
	assert.NoError(t, r.Body.Close())

	// get the rule by ID, should be marked as disabled
	b, code = get(t, ts.URL+"/api/rule/"+fmt.Sprintf(`%s`, ruleID))
	assert.Equal(t, http.StatusOK, code)
	rule = datastore.Rule{}
	err = json.Unmarshal([]byte(b), &rule)
	require.NoError(t, err)
	assert.Equal(t, randomDomainName, rule.Domain)
	assert.Equal(t, "test content", rule.Content)
	assert.False(t, rule.Enabled)

	// same disabled rule still should appear in All call
	b, code = get(t, ts.URL+"/api/rules")
	assert.Equal(t, http.StatusOK, code)
	rules = []datastore.Rule{}
	err = json.Unmarshal([]byte(b), &rules)
	require.NoError(t, err)
	assert.Contains(t, rules, rule)

	// get the disabled rule by domain, should not be found
	b, code = get(t, ts.URL+"/api/rule?url=https://"+randomDomainName)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "{\"error\":\"not found\"}\n", b)

	// save the rule with new content, ID should remain the same
	r, err = post(t, ts.URL+"/api/rule", fmt.Sprintf(`{"domain": "%s", "content": "new content"}`, randomDomainName))
	require.NoError(t, err)
	rule = datastore.Rule{}
	err = json.NewDecoder(r.Body).Decode(&rule)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, r.StatusCode)
	assert.NoError(t, r.Body.Close())
	assert.Equal(t, randomDomainName, rule.Domain)
	assert.Equal(t, "new content", rule.Content)
	assert.Equal(t, ruleID, rule.ID.Hex())
}

func TestServer_RuleUnhappyFlow(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	// save with wrong body
	r, err := post(t, ts.URL+"/api/rule", `""`)
	require.NoError(t, err)
	body, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.NoError(t, r.Body.Close())
	require.Equal(t, http.StatusInternalServerError, r.StatusCode)
	assert.Equal(t,
		"{\"error\":\"json: cannot unmarshal string into Go value of type datastore.Rule\"}\n",
		string(body))

	// get with no URL parameter set
	b, code := get(t, ts.URL+"/api/rule")
	assert.Equal(t, http.StatusExpectationFailed, code)
	assert.Equal(t, "{\"error\":\"no url passed\"}\n", b)

	// get rule by non-existent ID
	b, code = get(t, ts.URL+"/api/rule/nonexistent")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "{\"error\":\"not found\"}\n", b)
}

func TestServer_FakeAuth(t *testing.T) {
	ts, _ := startupT(t)
	defer ts.Close()

	// save with wrong body
	r, err := post(t, ts.URL+"/api/auth", `""`)
	require.NoError(t, err)
	body, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.NoError(t, r.Body.Close())
	assert.Equal(t, http.StatusOK, r.StatusCode)
	assert.Contains(t, string(body), `"pong":`)

	// get with no URL parameter set
	b, code := get(t, ts.URL+"/api/rule")
	assert.Equal(t, http.StatusExpectationFailed, code)
	assert.Equal(t, "{\"error\":\"no url passed\"}\n", b)

	// get rule by non-existent ID
	b, code = get(t, ts.URL+"/api/rule/nonexistent")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "{\"error\":\"not found\"}\n", b)
}

func get(t *testing.T, url string) (response string, statusCode int) {
	r, err := http.Get(url)
	assert.NoError(t, err)
	body, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.NoError(t, r.Body.Close())
	return string(body), r.StatusCode
}

func post(t *testing.T, url, body string) (*http.Response, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	assert.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	return client.Do(req)
}

func del(t *testing.T, url string) (*http.Response, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("DELETE", url, nil)
	assert.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	return client.Do(req)
}

// startupT runs fully configured testing server
func startupT(t *testing.T) (*httptest.Server, *Server) {
	if _, ok := os.LookupEnv("ENABLE_MONGO_TESTS"); !ok {
		t.Skip("ENABLE_MONGO_TESTS env variable is not set")
	}

	db, err := datastore.New("mongodb://localhost:27017/", "test_ureadability", 0)
	assert.NoError(t, err)
	srv := Server{
		Readability: extractor.UReadability{TimeOut: 30, SnippetSize: 300, Rules: db.GetStores()},
		Credentials: map[string]string{"admin": "password"},
		Version:     "dev-test",
	}

	return httptest.NewServer(srv.routes(".")), &srv
}

// thanks to https://stackoverflow.com/a/31832326/961092
func randStringBytesRmndr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int64()%int64(len(letterBytes))]
	}
	return string(b)
}
