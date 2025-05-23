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

var opts struct {
	Address     string            `long:"address" env:"UKEEPER_ADDRESS" default:"" description:"listening address"`
	Port        int               `long:"port" env:"UKEEPER_PORT" default:"8080" description:"port"`
	FrontendDir string            `long:"frontend-dir" env:"FRONTEND_DIR" default:"/srv/web" description:"directory with frontend templates and static/ directory for static assets"`
	Credentials map[string]string `long:"creds" env:"CREDS" description:"credentials for protected calls (POST, DELETE /rules)"`
	Token       string            `long:"token" env:"UKEEPER_TOKEN" description:"token for /content/v1/parser endpoint auth"`
	MongoURI    string            `short:"m" long:"mongo-uri" env:"MONGO_URI" required:"true" description:"MongoDB connection string"`
	MongoDelay  time.Duration     `long:"mongo-delay" env:"MONGO_DELAY" default:"0" description:"mongo initial delay"`
	MongoDB     string            `long:"mongo-db" env:"MONGO_DB" default:"ureadability" description:"mongo database name"`
	Debug       bool              `long:"dbg" env:"DEBUG" description:"debug mode"`
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
	srv := rest.Server{
		Readability: extractor.UReadability{
			TimeOut:     30 * time.Second,
			SnippetSize: 300,
			Rules:       stores.Rules,
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

	srv.Run(ctx, opts.Address, opts.Port, opts.FrontendDir)
}
