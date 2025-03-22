// Package main is the entry point for the ukeeper-readability service
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/ukeeper/ukeeper-readability/backend/datastore"
	"github.com/ukeeper/ukeeper-readability/backend/extractor"
	"github.com/ukeeper/ukeeper-readability/backend/rest"
)

var revision string

// OpenAIGroup contains settings for OpenAI integration
type OpenAIGroup struct {
	DisableSummaries  bool          `long:"disable-summaries" env:"DISABLE_SUMMARIES" description:"disable summary generation with OpenAI"`
	APIKey            string        `long:"api-key" env:"API_KEY" description:"OpenAI API key for summary generation"`
	ModelType         string        `long:"model-type" env:"MODEL_TYPE" default:"gpt-4o-mini" description:"OpenAI model name for summary generation (e.g., gpt-4o, gpt-4o-mini)"`
	SummaryPrompt     string        `long:"summary-prompt" env:"SUMMARY_PROMPT" description:"custom prompt for summary generation (default is used if not specified)"`
	MaxContentLength  int           `long:"max-content-length" env:"MAX_CONTENT_LENGTH" default:"10000" description:"maximum content length to send to OpenAI API (0 for no limit)"`
	RequestsPerMinute int           `long:"requests-per-minute" env:"REQUESTS_PER_MINUTE" default:"10" description:"maximum number of OpenAI API requests per minute (0 for no limit)"`
	CleanupInterval   time.Duration `long:"cleanup-interval" env:"CLEANUP_INTERVAL" default:"24h" description:"interval for cleaning up expired summaries"`
}

var opts struct {
	Address     string            `long:"address" env:"UKEEPER_ADDRESS" default:"" description:"listening address"`
	Port        int               `long:"port" env:"UKEEPER_PORT" default:"8080" description:"port"`
	FrontendDir string            `long:"frontend-dir" env:"FRONTEND_DIR" default:"/srv/web" description:"directory with frontend templates and static/ directory for static assets"`
	Credentials map[string]string `long:"creds" env:"CREDS" description:"credentials for protected calls (POST, DELETE /rules)"`
	Token       string            `long:"token" env:"UKEEPER_TOKEN" description:"token for /content/v1/parser endpoint auth"`
	MongoURI    string            `long:"mongo-uri" env:"MONGO_URI" required:"true" description:"MongoDB connection string"`
	MongoDelay  time.Duration     `long:"mongo-delay" env:"MONGO_DELAY" default:"0" description:"mongo initial delay"`
	MongoDB     string            `long:"mongo-db" env:"MONGO_DB" default:"ureadability" description:"mongo database name"`
	Debug       bool              `long:"dbg" env:"DEBUG" description:"debug mode"`

	OpenAI OpenAIGroup `group:"openai" namespace:"openai" env-namespace:"OPENAI" description:"OpenAI integration settings"`
}

func main() {
	if _, err := flags.Parse(&opts); err != nil {
		log.Printf("[ERROR] can't parse command line flags, %v", err)
		os.Exit(1)
	}
	var options []log.Option
	if opts.Debug {
		options = []log.Option{log.Debug, log.CallerFile}
	}
	options = append(options, log.Msec, log.LevelBraces)
	log.Setup(options...)

	log.Printf("[INFO] started ukeeper-readability service %s", revision)
	db, err := datastore.New(opts.MongoURI, opts.MongoDB, opts.MongoDelay)
	if err != nil {
		log.Fatalf("[ERROR] can't connect to mongo %v", err)
	}
	stores := db.GetStores()
	srv := rest.Server{
		Readability: extractor.UReadability{
			TimeOut:          30 * time.Second,
			SnippetSize:      300,
			Rules:            stores.Rules,
			Summaries:        stores.Summaries,
			OpenAIKey:        opts.OpenAI.APIKey,
			ModelType:        opts.OpenAI.ModelType,
			OpenAIEnabled:    !opts.OpenAI.DisableSummaries,
			SummaryPrompt:    opts.OpenAI.SummaryPrompt,
			MaxContentLength: opts.OpenAI.MaxContentLength,
			RequestsPerMin:   opts.OpenAI.RequestsPerMinute,
		},
		Token:       opts.Token,
		Credentials: opts.Credentials,
		Version:     revision,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { // catch signal and invoke graceful termination
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Printf("[WARN] interrupt signal")
		cancel()
	}()

	// start summary cleanup task if OpenAI is enabled
	if !opts.OpenAI.DisableSummaries {
		srv.Readability.StartCleanupTask(ctx, opts.OpenAI.CleanupInterval)
	}

	srv.Run(ctx, opts.Address, opts.Port, opts.FrontendDir)
}
