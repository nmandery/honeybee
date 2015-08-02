package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("Usage: honeybee [OPTIONS] [CONFIGURATION DIRECTORY]\n")
		fmt.Printf("\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	// imageproxy uses github.com/golang/glog an logs quite verbosely
	flag.Lookup("logtostderr").Value.Set("true")
	flag.Lookup("v").Value.Set("10")                  // this has no effect in imageproxys glog usage currently
	flag.Lookup("vmodule").Value.Set("imageproxy=10") // ditto

	// standard go logging
	log.SetOutput(os.Stderr)
}

func main() {
	args := flag.Args()
	if len(args) != 1 {
		fmt.Printf("Need exactly one argument specifying the configuration directory to use.\n")
		os.Exit(1)
	}

	var err error
	srv, err := NewServer(args[0])
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	srv.StartUpdating()
	err = srv.Serve()
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
