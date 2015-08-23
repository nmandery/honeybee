package main

import (
	_ "expvar"
	"flag"
	"fmt"
	"github.com/nmandery/honeybee"
	"log"
	"net/http"
	"os"
)

var expvarPort int = 0
var dropCache bool = false
var noServe bool = false

func init() {
	flag.Usage = func() {
		fmt.Printf("Usage: honeybee [OPTIONS] [CONFIGURATION DIRECTORY]\n")
		fmt.Printf("\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.IntVar(&expvarPort, "expvar-port", 0, "Port to provide the expvar interface on. Disabled per default (0).")
	flag.BoolVar(&dropCache, "drop-cache", false, "Drop the cache before starting the server.")
	flag.BoolVar(&noServe, "no-serve", false, "Do not run the server")
	flag.Parse()

	// standard go logging
	log.SetOutput(os.Stderr)
}

func main() {
	args := flag.Args()
	if len(args) != 1 {
		fmt.Printf("Need exactly one argument specifying the configuration directory to use.\n")
		os.Exit(1)
	}

	if expvarPort != 0 {
		go func() {
			log.Printf("Serving expvar statistics on port %d\n", expvarPort)
			experr := http.ListenAndServe(fmt.Sprintf(":%d", expvarPort), nil)
			if experr != nil {
				log.Printf("Could not serve expvar stats on port %d: %v\n", expvarPort, experr)
				os.Exit(1)
			}
		}()
	}

	var err error
	srv, err := honeybee.NewServer(args[0])
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	if dropCache {
		log.Printf("Dropping the cache ...")
		srv.DropCache()
		log.Println("Cache dropped.")
	}

	if noServe == false {
		srv.StartUpdating()
		err = srv.Serve()
		if err != nil {
			log.Printf("%v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(0)
}
