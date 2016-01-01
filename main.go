package main

import (
	"log"
	"os"

	"umputun.com/ureadability/extractor"
	"umputun.com/ureadability/rest"

	"github.com/jessevdk/go-flags"
)

var opts struct {
	// Mongo            string `short:"m" long:"mongo" env:"MONGO" description:"mongo host:port"`
	Migrate bool `long:"migrate" default:"false" description:"enable migration"`
}

func main() {
	if _, err := flags.Parse(&opts); err != nil {
		os.Exit(1)
	}

	rest.Server{Readability: extractor.UReadability{TimeOut: 30, SnippetSize: 300}}.Run()
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile | log.Lmicroseconds)
}
