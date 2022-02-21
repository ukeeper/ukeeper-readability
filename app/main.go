package main

import (
	"os"

	log "github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/ukeeper/ukeeper-redabilty/app/datastore"
	"github.com/ukeeper/ukeeper-redabilty/app/extractor"
	"github.com/ukeeper/ukeeper-redabilty/app/rest"
)

var revision string

var opts struct {
	Mongo       string            `short:"m" long:"mongo" env:"MONGO" description:"mongo host:port"`
	MongoPasswd string            `short:"p" long:"mongo-password" env:"MONGO_PASSWD" default:"" description:"mongo pssword"`
	MongoDelay  int               `long:"mongo-delay" env:"MONGO_DELAY" default:"0" description:"mongo initial delay"`
	MongoDB     string            `long:"mongo-db" env:"MONGO_DB" default:"ureadability" description:"mongo database name"`
	Migrate     bool              `long:"migrate" description:"enable migration"`
	Credentials map[string]string `long:"creds" env:"CREDS" description:"credentials for protected calls"`
	Debug       bool              `long:"dbg" env:"DEBUG" description:"debug mode"`
}

func main() {
	if _, err := flags.Parse(&opts); err != nil {
		os.Exit(1)
	}
	setupLog(opts.Debug)

	log.Printf("[INFO] started ureadability service, %s", revision)
	rules := datastore.New(opts.Mongo, opts.MongoPasswd, opts.MongoDB, opts.MongoDelay).GetStores()
	srv := rest.Server{
		Readability: extractor.UReadability{TimeOut: 30, SnippetSize: 300, Rules: rules, Debug: opts.Debug},
		Credentials: opts.Credentials,
		Version:     revision,
	}
	srv.Run()
}

func setupLog(dbg bool) {
	if dbg {
		log.Setup(log.Debug, log.CallerFile, log.Msec, log.LevelBraces)
		return
	}
	log.Setup(log.Msec, log.LevelBraces)
}
