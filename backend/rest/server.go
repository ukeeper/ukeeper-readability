// Package rest implement http server with API
package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/didip/tollbooth/v6"
	"github.com/didip/tollbooth_chi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	log "github.com/go-pkgz/lgr"
	UM "github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/ukeeper/ukeeper-redabilty/backend/datastore"
	"github.com/ukeeper/ukeeper-redabilty/backend/extractor"
)

// Server is a basic rest server providing access to store and invoking parser
type Server struct {
	Readability extractor.UReadability
	Version     string
	Credentials map[string]string
}

// JSON is a map alias, just for convenience
type JSON map[string]interface{}

// Run the listen and request's router, activate rest server
func (s Server) Run(ctx context.Context, address string, port int, frontendDir string) {
	log.Printf("[INFO] activate rest server on %s:%d", address, port)

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

func (s Server) routes(frontendDir string) chi.Router {
	router := chi.NewRouter()

	router.Use(middleware.RequestID, middleware.RealIP, UM.Recoverer(log.Default()))
	router.Use(middleware.Throttle(1000), middleware.Timeout(60*time.Second))
	router.Use(UM.AppInfo("ureadability", "Umputun", s.Version), UM.Ping)
	router.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(50, nil)))

	router.Use(logger.New(logger.Log(log.Default()), logger.WithBody, logger.Prefix("[INFO]")).Handler)

	router.Route("/api", func(r chi.Router) {
		r.Get("/content/v1/parser", s.extractArticleEmulateReadability)
		r.Post("/extract", s.extractArticle)

		r.Get("/rule", s.GetRule)
		r.Get("/rule/{id}", s.GetRuleByID)
		r.Get("/rules", s.GetAllRules)
		r.Post("/auth", s.AuthFake)

		r.Group(func(protected chi.Router) {
			basicAuth("ureadability", s.Credentials)
			protected.Post("/rule", s.SaveRule)
			protected.Delete("/rule/{id}", s.DeleteRule)
		})
	})

	fs, err := UM.NewFileServer("/", frontendDir, UM.FsOptSPA)
	if err != nil {
		log.Fatalf("unable to create, %v", err)
	}
	router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	})
	return router
}

func (s Server) extractArticle(w http.ResponseWriter, r *http.Request) {
	artRequest := extractor.Response{}
	if err := render.DecodeJSON(r.Body, &artRequest); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, JSON{"error": err.Error()})
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

// emulate readability API parse - https://www.readability.com/api/content/v1/parser?token=%s&url=%s
func (s Server) extractArticleEmulateReadability(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		render.Status(r, http.StatusExpectationFailed)
		render.JSON(w, r, JSON{"error": "no token passed"})
		return
	}

	url := r.URL.Query().Get("url")
	if url == "" {
		render.Status(r, http.StatusExpectationFailed)
		render.JSON(w, r, JSON{"error": "no url passed"})
		return
	}

	res, err := s.Readability.Extract(r.Context(), url)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": err.Error()})
		return
	}

	render.JSON(w, r, &res)
}

// GetRule find rule matching url param (domain portion only)
func (s Server) GetRule(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		render.Status(r, http.StatusExpectationFailed)
		render.JSON(w, r, JSON{"error": "no url passed"})
		return
	}

	rule, found := s.Readability.Rules.Get(r.Context(), url)
	if !found {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": "not found"})
		return
	}

	log.Printf("[DEBUG] rule for %s found, %v", url, rule)
	render.JSON(w, r, rule)
}

// GetRuleByID returns rule by id - GET /rule/:id"
func (s Server) GetRuleByID(w http.ResponseWriter, r *http.Request) {
	id := getBid(chi.URLParam(r, "id"))
	rule, found := s.Readability.Rules.GetByID(r.Context(), id)
	if !found {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": "not found"})
		return
	}
	log.Printf("[DEBUG] rule for %s found, %v", id.Hex(), rule)
	render.JSON(w, r, &rule)
}

// GetAllRules returns list of all rules, including disabled
func (s Server) GetAllRules(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, s.Readability.Rules.All(r.Context()))
}

// SaveRule upsert rule, forcing enabled=true
func (s Server) SaveRule(w http.ResponseWriter, r *http.Request) {
	rule := datastore.Rule{}

	if err := render.DecodeJSON(r.Body, &rule); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, JSON{"error": err.Error()})
		return
	}

	rule.Enabled = true
	srule, err := s.Readability.Rules.Save(r.Context(), rule)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": err.Error()})
		return
	}
	render.JSON(w, r, &srule)
}

// DeleteRule marks rule as disabled
func (s Server) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id := getBid(chi.URLParam(r, "id"))
	err := s.Readability.Rules.Disable(r.Context(), id)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, JSON{"error": err.Error()})
		return
	}
	render.JSON(w, r, JSON{"disabled": id})
}

// AuthFake just a dummy post request used for external check for protected resource
func (s Server) AuthFake(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Add("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, realm))
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
			if !userFound {
				unauthorized(w, realm)
				return
			}
			if password != validPassword {
				unauthorized(w, realm)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
