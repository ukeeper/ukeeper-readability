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
	"strings"
	"time"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth_chi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	log "github.com/go-pkgz/lgr"
	um "github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/ukeeper/ukeeper-redabilty/backend/datastore"
	"github.com/ukeeper/ukeeper-redabilty/backend/extractor"
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

func (s *Server) routes(frontendDir string) chi.Router {
	router := chi.NewRouter()

	router.Use(middleware.RequestID, middleware.RealIP, um.Recoverer(log.Default()))
	router.Use(middleware.Throttle(1000), middleware.Timeout(60*time.Second))
	router.Use(um.AppInfo("ureadability", "Umputun", s.Version), um.Ping)
	router.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(50, nil)))

	router.Use(logger.New(logger.Log(log.Default()), logger.WithBody, logger.Prefix("[INFO]")).Handler)

	router.Route("/api", func(r chi.Router) {
		r.Get("/content/v1/parser", s.extractArticleEmulateReadability)
		r.Post("/extract", s.extractArticle)
		r.Post("/auth", s.authFake)

		r.Group(func(protected chi.Router) {
			protected.Use(basicAuth("ureadability", s.Credentials))
			protected.Post("/rule", s.saveRule)
			protected.Post("/toggle-rule/{id}", s.toggleRule)
			protected.Post("/preview", s.handlePreview)
		})
	})

	router.Get("/", s.handleIndex)
	router.Get("/add/", s.handleAdd)
	router.Get("/edit/{id}", s.handleEdit)

	_ = os.Mkdir(filepath.Join(frontendDir, "static"), 0o700)
	fs, err := um.NewFileServer("/", filepath.Join(frontendDir, "static"), um.FsOptSPA)
	if err != nil {
		log.Printf("[ERROR] unable to create file server, %v", err)
		return nil
	}
	router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	})
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
	id := getBid(chi.URLParam(r, "id"))
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
	if err := render.DecodeJSON(r.Body, &artRequest); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, JSON{"error": err.Error()})
		return
	}

	if artRequest.URL == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": "url parameter is required"})
		return
	}

	res, err := s.Readability.Extract(r.Context(), artRequest.URL)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": err.Error()})
		return
	}

	render.JSON(w, r, &res)
}

// extractArticleEmulateReadability emulates readability API parse - https://www.readability.com/api/content/v1/parser?token=%s&url=%s
// if token is not set for application, it won't be checked
func (s *Server) extractArticleEmulateReadability(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if s.Token != "" && token == "" {
		render.Status(r, http.StatusExpectationFailed)
		render.JSON(w, r, JSON{"error": "no token passed"})
		return
	}

	if s.Token != "" && s.Token != token {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, JSON{"error": "wrong token passed"})
		return
	}

	extractURL := r.URL.Query().Get("url")
	if extractURL == "" {
		render.Status(r, http.StatusExpectationFailed)
		render.JSON(w, r, JSON{"error": "no url passed"})
		return
	}

	res, err := s.Readability.Extract(r.Context(), extractURL)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": err.Error()})
		return
	}

	render.JSON(w, r, &res)
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

		responses = append(responses, *result)
	}

	// create a new type where Rich would be type template.HTML instead of string,
	// to avoid escaping in the template
	type result struct {
		Title   string
		Excerpt string
		Rich    template.HTML
		Content string
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
	render.JSON(w, r, &srule)
}

func (s *Server) toggleRule(w http.ResponseWriter, r *http.Request) {
	id := getBid(chi.URLParam(r, "id"))
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
func (s *Server) authFake(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	render.JSON(w, r, JSON{"pong": t.Format("20060102150405")})
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
