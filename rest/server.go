package rest

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"umputun.com/ukeeper/ureadability/extractor"
)

//Server basic rest server to access msgs from mongo
type Server struct {
	Readability extractor.UReadability
}

//Run the lister and request's router, activate rest server
func (r Server) Run() {
	log.Printf("activate rest server")

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		t := time.Now()
		c.Next()
		log.Printf("%s %s %s %v %d", c.Request.Method, c.Request.URL.Path, c.ClientIP(), time.Since(t), c.Writer.Status())
	})

	router.POST("/api/v1/extract", r.extractArticle)
	router.GET("/api/content/v1/parser", r.extractArticleEmulateReadability)

	log.Fatal(router.Run(":8080"))
}

func (r Server) extractArticle(c *gin.Context) {

	artRequest := extractor.Response{}

	err := c.BindJSON(&artRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	res, err := r.Readability.Extract(artRequest.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

//emulate radability API parse - https://www.readability.com/api/content/v1/parser?token=%s&url=%s
func (r Server) extractArticleEmulateReadability(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusExpectationFailed, gin.H{"error": "no token passed"})
		return
	}
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusExpectationFailed, gin.H{"error": "no url passed"})
		return
	}
	res, err := r.Readability.Extract(url)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
