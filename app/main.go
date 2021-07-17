package main

import (
	"log"
	"os"

	"ukeeper.com/ureadability/app/datastore"
	"ukeeper.com/ureadability/app/extractor"
	"ukeeper.com/ureadability/app/rest"

	"github.com/jessevdk/go-flags"
)

var gitRevision string

var opts struct {
	Mongo       string `short:"m" long:"mongo" env:"MONGO" description:"mongo host:port"`
	MongoPasswd string `short:"p" long:"mongo-password" env:"MONGO_PASSWD" default:"" description:"mongo password"`
	MongoDelay  int    `long:"mongo-delay" env:"MONGO_DELAY" default:"0" description:"mongo initial delay"`
	MongoDB     string `long:"mongo-db" env:"MONGO_DB" default:"ureadability" description:"mongo database name"`
	Debug       bool   `long:"dbg" env:"DEBUG" description:"debug mode"`
	Migrate     bool   `long:"migrate" description:"enable migration"`
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile | log.Lmicroseconds)
	if _, err := flags.Parse(&opts); err != nil {
		os.Exit(1)
	}
	log.Printf("started ureadability servcie, %s", gitRevision)
	rules := datastore.New(opts.Mongo, opts.MongoPasswd, opts.MongoDB, opts.MongoDelay).GetStores()
	rest.Server{Readability: extractor.UReadability{TimeOut: 30, SnippetSize: 300, Rules: rules, Debug: opts.Debug}}.Run()
}
