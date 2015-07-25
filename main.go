package main

import (
	"flag"
	"log"
	"os"
)

func init() {
	flag.Parse()
	// imageproxy uses github.com/golang/glog an logs quite verbosely
	flag.Lookup("logtostderr").Value.Set("true")
	flag.Lookup("v").Value.Set("10")                  // this has no effect in imageproxys glog usage currently
	flag.Lookup("vmodule").Value.Set("imageproxy=10") // ditto

	// standard go logging
	log.SetOutput(os.Stderr)
}

func main() {

	var err error
	srv, err := NewServer(os.Args[1])
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
