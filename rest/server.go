package rest

import (
	"log"
	"net/http"
	"sync"

	"github.com/ant0ine/go-json-rest/rest"
	"umputun.com/ureadability/extractor"
)

//Server basic rest server to access msgs from mongo
type Server struct {
	Readability extractor.UReadability
	updLock     *sync.Mutex
	activeLock  *sync.RWMutex
}

//Run the lister and request's router, activate rest server
func (r Server) Run() {
	log.Printf("activate rest server")

	api := rest.NewApi()
	api.Use(rest.DefaultCommonStack...)

	router, err := rest.MakeRouter(
		rest.Post("/api/v1/extract", r.extractArticle),
		rest.Get("/api/content/v1/parser", r.extractArticleEmulateReadability),
	)

	if err != nil {
		log.Fatal(err)
	}
	api.SetApp(router)
	log.Fatal(http.ListenAndServe(":8080", api.MakeHandler()))
}

func (r Server) extractArticle(w rest.ResponseWriter, req *rest.Request) {

	artRequest := extractor.Response{}
	err := req.DecodeJsonPayload(&artRequest)
	if err != nil {
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res, err := r.Readability.Extract(artRequest.URL)
	if err != nil {
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteJson(res)
}

//emulate radability API parse - https://www.readability.com/api/content/v1/parser?token=%s&url=%s
func (r Server) extractArticleEmulateReadability(w rest.ResponseWriter, req *rest.Request) {
	token := req.URL.Query().Get("token")
	if token == "" {
		rest.Error(w, "no token passed", http.StatusExpectationFailed)
		return
	}
	url := req.URL.Query().Get("url")
	if url == "" {
		rest.Error(w, "no url passed", http.StatusExpectationFailed)
		return
	}
	res, err := r.Readability.Extract(url)
	if err != nil {
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteJson(res)

}
