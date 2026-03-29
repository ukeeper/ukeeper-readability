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

	"github.com/ukeeper/ukeeper-readability/datastore"
	"github.com/ukeeper/ukeeper-readability/extractor"
	"github.com/ukeeper/ukeeper-readability/rest"
)

var revision string

// OpenAIGroup contains settings for OpenAI integration
type OpenAIGroup struct {
	DisableSummaries  bool          `long:"disable-summaries" env:"DISABLE_SUMMARIES" description:"disable summary generation with OpenAI"`
	APIKey            string        `long:"api-key" env:"API_KEY" description:"OpenAI API key for summary generation"`
	ModelType         string        `long:"model-type" env:"MODEL_TYPE" default:"gpt-4o-mini" description:"OpenAI model name for summary generation (e.g., gpt-4o, gpt-4o-mini)"`
	SummaryPrompt     string        `long:"summary-prompt" env:"SUMMARY_PROMPT" description:"custom prompt for summary generation"`
	MaxContentLength  int           `long:"max-content-length" env:"MAX_CONTENT_LENGTH" default:"10000" description:"maximum content length to send to OpenAI API (0 for no limit)"`
	RequestsPerMinute int           `long:"requests-per-minute" env:"REQUESTS_PER_MINUTE" default:"10" description:"maximum number of OpenAI API requests per minute (0 for no limit)"`
	CleanupInterval   time.Duration `long:"cleanup-interval" env:"CLEANUP_INTERVAL" default:"24h" description:"interval for cleaning up expired summaries"`
}

var opts struct {
	Address      string            `long:"address" env:"UKEEPER_ADDRESS" default:"" description:"listening address"`
	Port         int               `long:"port" env:"UKEEPER_PORT" default:"8080" description:"port"`
	FrontendDir  string            `long:"frontend-dir" env:"FRONTEND_DIR" default:"/srv/web" description:"directory with frontend templates and static/ directory for static assets"`
	Credentials  map[string]string `long:"creds" env:"CREDS" description:"credentials for protected calls (POST, DELETE /rules)"`
	Token        string            `long:"token" env:"UKEEPER_TOKEN" description:"token for API endpoint auth"`
	MongoURI     string            `short:"m" long:"mongo-uri" env:"MONGO_URI" required:"true" description:"MongoDB connection string"`
	MongoDelay   time.Duration     `long:"mongo-delay" env:"MONGO_DELAY" default:"0" description:"mongo initial delay"`
	MongoDB      string            `long:"mongo-db" env:"MONGO_DB" default:"ureadability" description:"mongo database name"`
	CFAccountID  string            `long:"cf-account-id" env:"CF_ACCOUNT_ID" description:"Cloudflare account ID for Browser Rendering API"`
	CFAPIToken   string            `long:"cf-api-token" env:"CF_API_TOKEN" description:"Cloudflare API token with Browser Rendering Edit permission"`
	OpenAIKey    string            `long:"openai-api-key" env:"OPENAI_API_KEY" description:"OpenAI API key; enables auto-evaluation when set"`
	OpenAIModel  string            `long:"openai-model" env:"OPENAI_MODEL" default:"gpt-5.4-mini" description:"OpenAI model for evaluation"`
	OpenAIMaxItr int               `long:"openai-max-iter" env:"OPENAI_MAX_ITER" default:"3" description:"max evaluation iterations per extraction"`
	Debug        bool              `long:"dbg" env:"DEBUG" description:"debug mode"`

	OpenAI OpenAIGroup `group:"openai" namespace:"openai" env-namespace:"OPENAI" description:"OpenAI integration settings"`
}

func main() {
	if _, err := flags.Parse(&opts); err != nil {
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

	var retriever extractor.Retriever
	if opts.CFAccountID != "" && opts.CFAPIToken != "" {
		retriever = &extractor.CloudflareRetriever{
			AccountID: opts.CFAccountID,
			APIToken:  opts.CFAPIToken,
			Timeout:   30 * time.Second,
		}
		log.Printf("[INFO] using Cloudflare Browser Rendering retriever, account=%s", opts.CFAccountID)
	} else {
		if opts.CFAccountID != "" || opts.CFAPIToken != "" {
			log.Print("[WARN] both --cf-account-id and --cf-api-token must be set for Cloudflare Browser Rendering; falling back to default HTTP retriever")
		}
		retriever = &extractor.HTTPRetriever{Timeout: 30 * time.Second}
		log.Print("[INFO] using default HTTP retriever")
	}

	// determine the OpenAI API key — use the dedicated summary key if set, fall back to the evaluation key
	openAIKeyForSummaries := opts.OpenAI.APIKey
	if openAIKeyForSummaries == "" {
		openAIKeyForSummaries = opts.OpenAIKey
	}

	srv := rest.Server{
		Readability: extractor.UReadability{
			TimeOut:          30 * time.Second,
			SnippetSize:      300,
			Rules:            stores.Rules,
			Retriever:        retriever,
			Summaries:        stores.Summaries,
			OpenAIKey:        openAIKeyForSummaries,
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

	if opts.OpenAIKey != "" {
		srv.Readability.AIEvaluator = &extractor.OpenAIEvaluator{
			APIKey: opts.OpenAIKey,
			Model:  opts.OpenAIModel,
		}
		srv.Readability.MaxGPTIter = opts.OpenAIMaxItr
		log.Printf("[INFO] OpenAI evaluation enabled, model=%s, max-iter=%d", opts.OpenAIModel, opts.OpenAIMaxItr)
	} else {
		log.Print("[INFO] OpenAI evaluation disabled (no API key)")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { // catch signal and invoke graceful termination
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Print("[WARN] interrupt signal")
		cancel()
	}()

	// start summary cleanup task if openai is enabled
	if !opts.OpenAI.DisableSummaries {
		srv.Readability.StartCleanupTask(ctx, opts.OpenAI.CleanupInterval)
	}

	srv.Run(ctx, opts.Address, opts.Port, opts.FrontendDir)
}
