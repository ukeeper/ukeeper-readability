package main

import (
	"log"
	"os"

	"umputun.com/ukeeper/ureadability/datastore"
	"umputun.com/ukeeper/ureadability/extractor"
	"umputun.com/ukeeper/ureadability/rest"

	"github.com/jessevdk/go-flags"
)

var gitRevision string

var opts struct {
	Mongo       string `short:"m" long:"mongo" env:"MONGO" description:"mongo host:port"`
	MongoPasswd string `short:"p" long:"mongo-password" env:"MONGO_PASSWD" default:"" description:"mongo pssword"`
	MongoDelay  int    `long:"mongo-delay" env:"MONGO_DELAY" default:"0" description:"mongo initial delay"`
	MongoDB     string `long:"mongo-db" env:"MONGO_DB" default:"ureadability" description:"mongo database name"`
	Debug       bool   `long:"dbg" env:"DEBUG" default:"false" description:"debug mode"`
	Migrate     bool   `long:"migrate" default:"false" description:"enable migration"`
}

func main() {
	if _, err := flags.Parse(&opts); err != nil {
		os.Exit(1)
	}
	log.Printf("started ureadability servcie, %s", gitRevision)
	rules := datastore.New(opts.Mongo, opts.MongoPasswd, opts.MongoDB, opts.MongoDelay).GetStores()
	rest.Server{Readability: extractor.UReadability{TimeOut: 30, SnippetSize: 300, Rules: rules, Debug: opts.Debug}}.Run()
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile | log.Lmicroseconds)
}
