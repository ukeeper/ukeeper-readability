// Package rest implement http server with API
package rest

import (
	"context"
	"crypto/subtle"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/go-pkgz/lgr"
	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"github.com/go-pkgz/routegroup"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/ukeeper/ukeeper-readability/backend/datastore"
	"github.com/ukeeper/ukeeper-readability/backend/extractor"
)

// Server is a basic rest server providing access to store and invoking parser
type Server struct {
	Readability extractor.UReadability
	Version     string
	Token       string
	Credentials map[string]string

	indexPage *template.Template
	rulePage  *template.Template
}

// JSON is a map alias, just for convenience
type JSON map[string]any

// Run the listen and request's router, activate rest server
func (s *Server) Run(ctx context.Context, address string, port int, frontendDir string) {
	log.Printf("[INFO] activate rest server on %s:%d", address, port)

	_ = os.Mkdir(filepath.Join(frontendDir, "components"), 0o700)
	t := template.Must(template.ParseGlob(filepath.Join(frontendDir, "components", "*.gohtml")))
	s.rulePage = template.Must(template.Must(t.Clone()).ParseFiles(filepath.Join(frontendDir, "rule.gohtml")))
	s.indexPage = template.Must(template.Must(t.Clone()).ParseFiles(filepath.Join(frontendDir, "index.gohtml")))
	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", address, port),
		Handler:           s.routes(frontendDir),
		ReadHeaderTimeout: 5 * time.Second,
		// WriteTimeout:      120 * time.Second, // TODO: such a long timeout needed for blocking export (backup) request
		IdleTimeout: 30 * time.Second,
	}
	go func() {
		// shutdown on context cancellation
		<-ctx.Done()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("[DEBUG] http shutdown error, %s", err)
		}
		log.Print("[DEBUG] http server shutdown completed")
	}()

	log.Printf("[WARN] http server terminated, %s", httpServer.ListenAndServe())
}

func (s *Server) routes(frontendDir string) http.Handler {
	router := routegroup.New(http.NewServeMux())

	router.Use(rest.Recoverer(log.Default()))
	router.Use(rest.RealIP)
	router.Use(rest.AppInfo("ureadability", "Umputun", s.Version), rest.Ping)
	router.Use(rest.Throttle(50))
	router.Use(logger.New(logger.Log(log.Default()), logger.WithBody, logger.Prefix("[INFO]")).Handler)

	router.Route(func(api *routegroup.Bundle) {
		api.Mount("/api").Route(func(api *routegroup.Bundle) {
			api.HandleFunc("GET /content/v1/parser", s.extractArticleEmulateReadability)
			api.HandleFunc("POST /extract", s.extractArticle)
			api.HandleFunc("POST /auth", s.authFake)

			// add protected group with its own set of middlewares
			protectedGroup := api.Group()
			protectedGroup.Use(basicAuth("ureadability", s.Credentials))
			protectedGroup.HandleFunc("POST /rule", s.saveRule)
			protectedGroup.HandleFunc("POST /toggle-rule/{id}", s.toggleRule)
			protectedGroup.HandleFunc("POST /preview", s.handlePreview)
		})
	})

	router.HandleFunc("GET /", s.handleIndex)
	router.HandleFunc("GET /add/", s.handleAdd)
	router.HandleFunc("GET /edit/{id}", s.handleEdit)

	_ = os.Mkdir(filepath.Join(frontendDir, "static"), 0o700)
	router.HandleFiles("/", http.Dir(filepath.Join(frontendDir, "static")))
	return router
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	rules := s.Readability.Rules.All(r.Context())
	data := struct {
		Title string
		Rules []datastore.Rule
	}{
		Title: "Правила",
		Rules: rules,
	}
	err := s.indexPage.ExecuteTemplate(w, "base.gohtml", data)
	if err != nil {
		log.Printf("[WARN] failed to render index template, %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleAdd(w http.ResponseWriter, _ *http.Request) {
	data := struct {
		Title string
		Rule  datastore.Rule
	}{
		Title: "Добавление правила",
		Rule:  datastore.Rule{}, // empty rule for the form
	}
	err := s.rulePage.ExecuteTemplate(w, "base.gohtml", data)
	if err != nil {
		log.Printf("[WARN] failed to render add template, %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	id := getBid(r.PathValue("id"))
	rule, found := s.Readability.Rules.GetByID(r.Context(), id)
	if !found {
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}
	data := struct {
		Title string
		Rule  datastore.Rule
	}{
		Title: "Редактирование правила",
		Rule:  rule,
	}
	err := s.rulePage.ExecuteTemplate(w, "base.gohtml", data)
	if err != nil {
		log.Printf("[WARN] failed to render edit template, %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) extractArticle(w http.ResponseWriter, r *http.Request) {
	artRequest := extractor.Response{}
	if err := rest.DecodeJSON(r, &artRequest); err != nil {
		rest.SendErrorJSON(w, r, log.Default(), http.StatusInternalServerError, err, "can't parse request")
		return
	}

	if artRequest.URL == "" {
		rest.SendErrorJSON(w, r, log.Default(), http.StatusBadRequest, nil, "url parameter is required")
		return
	}

	res, err := s.Readability.Extract(r.Context(), artRequest.URL)
	if err != nil {
		rest.SendErrorJSON(w, r, log.Default(), http.StatusBadRequest, err, "can't extract content")
		return
	}

	rest.RenderJSON(w, &res)
}

// extractArticleEmulateReadability emulates readability API parse - https://www.readability.com/api/content/v1/parser?token=%s&url=%s
// if token is not set for application, it won't be checked
func (s *Server) extractArticleEmulateReadability(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	summary, _ := strconv.ParseBool(r.URL.Query().Get("summary"))

	if s.Token != "" && token == "" {
		rest.SendErrorJSON(w, r, log.Default(), http.StatusExpectationFailed, nil, "no token passed")
		return
	}

	// Check if summary is requested but token is not provided, or OpenAI key is not set
	if summary {
		if s.Readability.OpenAIKey == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, JSON{"error": "OpenAI key is not set"})
			return
		}
		if s.Token == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, JSON{"error": "summary generation requires token, but token is not set for the server"})
			return
		}
	}

	if s.Token != "" && s.Token != token {
		rest.SendErrorJSON(w, r, log.Default(), http.StatusUnauthorized, nil, "wrong token passed")
		return
	}

	extractURL := r.URL.Query().Get("url")
	if extractURL == "" {
		rest.SendErrorJSON(w, r, log.Default(), http.StatusExpectationFailed, nil, "no url passed")
		return
	}

	res, err := s.Readability.Extract(r.Context(), extractURL)
	if err != nil {
		rest.SendErrorJSON(w, r, log.Default(), http.StatusBadRequest, err, "can't extract content")
		return
	}

	if summary {
		summaryText, err := s.Readability.GenerateSummary(r.Context(), res.Content)
		if err != nil {
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, JSON{"error": fmt.Sprintf("failed to generate summary: %v", err)})
			return
		}
		res.Summary = summaryText
	}

	rest.RenderJSON(w, &res)
}

// generates previews for the provided test URLs
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	testURLs := strings.Split(r.FormValue("test_urls"), "\n")
	content := strings.TrimSpace(r.FormValue("content"))
	log.Printf("[INFO] test urls: %v", testURLs)
	log.Printf("[INFO] custom rule: %v", content)

	// create a temporary rule for extraction
	var tempRule *datastore.Rule
	if content != "" {
		tempRule = &datastore.Rule{
			Enabled: true,
			Content: content,
		}
	}

	responses := make([]extractor.Response, 0, len(testURLs))
	for _, url := range testURLs {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}

		log.Printf("[DEBUG] custom rule provided for %s: %v", url, tempRule)
		result, e := s.Readability.ExtractByRule(r.Context(), url, tempRule)
		if e != nil {
			log.Printf("[WARN] failed to extract content for %s: %v", url, e)
			continue
		}

		if s.Readability.OpenAIKey != "" {
			result.Summary, e = s.Readability.GenerateSummary(r.Context(), result.Content)
			if e != nil {
				log.Printf("[WARN] failed to generate summary for preview of %s: %v", url, e)
			}
		}

		responses = append(responses, *result)
	}

	// create a new type where Rich would be type template.HTML instead of string,
	// to avoid escaping in the template
	type result struct {
		Title   string
		Excerpt string
		Rich    template.HTML
		Content string
		Summary template.HTML
	}

	results := make([]result, 0, len(responses))
	for i := range responses {
		r := &responses[i]
		results = append(results, result{
			Title:   r.Title,
			Excerpt: r.Excerpt,
			//nolint:gosec // this content is escaped by Extractor, so it's safe to use it as is
			Rich:    template.HTML(r.Rich),
			Content: r.Content,
			//nolint: gosec // we do not expect CSS from OpenAI response
			Summary: template.HTML(strings.ReplaceAll(r.Summary, "\n", "<br>")),
		})
	}

	data := struct {
		Results []result
	}{
		Results: results,
	}

	err = s.rulePage.ExecuteTemplate(w, "preview.gohtml", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// saveRule upsert rule, forcing enabled=true
func (s *Server) saveRule(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}
	rule := datastore.Rule{
		Enabled:   true,
		ID:        getBid(r.FormValue("id")),
		Domain:    r.FormValue("domain"),
		Author:    r.FormValue("author"),
		Content:   r.FormValue("content"),
		MatchURLs: strings.Split(r.FormValue("match_url"), "\n"),
		Excludes:  strings.Split(r.FormValue("excludes"), "\n"),
		TestURLs:  strings.Split(r.FormValue("test_urls"), "\n"),
	}

	// return error in case domain is not set
	if rule.Domain == "" {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	srule, err := s.Readability.Rules.Save(r.Context(), rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	rest.RenderJSON(w, &srule)
}

func (s *Server) toggleRule(w http.ResponseWriter, r *http.Request) {
	id := getBid(r.PathValue("id"))
	rule, found := s.Readability.Rules.GetByID(r.Context(), id)
	if !found {
		log.Printf("[WARN] rule not found for id: %s", id.Hex())
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}

	rule.Enabled = !rule.Enabled
	var err error
	if rule.Enabled {
		_, err = s.Readability.Rules.Save(r.Context(), rule)
	} else {
		err = s.Readability.Rules.Disable(r.Context(), id)
	}

	if err != nil {
		log.Printf("[ERROR] failed to toggle rule: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = s.indexPage.ExecuteTemplate(w, "rule-row", rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// authFake just a dummy post request used for external check for protected resource
func (s *Server) authFake(w http.ResponseWriter, _ *http.Request) {
	t := time.Now()
	rest.RenderJSON(w, JSON{"pong": t.Format("20060102150405")})
}

func getBid(id string) primitive.ObjectID {
	bid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return primitive.NilObjectID
	}
	return bid
}

// basicAuth returns a piece of middleware that will allow access only
// if the provided credentials match within the given service
// otherwise, it will return a 401 and not call the next handler.
// source: https://github.com/99designs/basicauth-go/blob/master/basicauth.go
func basicAuth(realm string, credentials map[string]string) func(http.Handler) http.Handler {
	unauthorized := func(w http.ResponseWriter, realm string) {
		w.Header().Add("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", realm))
		w.WriteHeader(http.StatusUnauthorized)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok {
				unauthorized(w, realm)
				return
			}

			validPassword, userFound := credentials[username]
			validPasswordBytes := []byte(validPassword)
			if !userFound {
				unauthorized(w, realm)
				return
			}
			// take the same amount of time if the lengths are different
			// this is required since ConstantTimeCompare returns immediately when slices of different length are compared
			if len(password) != len(validPassword) {
				subtle.ConstantTimeCompare(validPasswordBytes, validPasswordBytes)
			}
			if subtle.ConstantTimeCompare([]byte(password), validPasswordBytes) == 0 {
				unauthorized(w, realm)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
