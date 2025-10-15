/*
 * Copyright Datadatdat.
 */

package main

import (
	"flag"
	"fmt"
	"github.com/datadatdat/datadatdat-docker-proxy/internal/forwarder"
	"github.com/datadatdat/datadatdat-docker-proxy/internal/listener"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: docker-volume-forwarder [--host host] [--port port] socket\n")
		flag.PrintDefaults()
	}

	host := flag.String("host", "localhost", "host to connect to")
	port := flag.Int("port", 5001, "port to connect to")

	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "missing required socket path")
		os.Exit(2)
	}
	path := flag.Arg(0)

	fmt.Printf("Proxying requests from %s to %s:%d\n", path, *host, *port)

	forward := forwarder.New(*host, *port)
	listen := listener.New(forward, path)
	listen.SetLogging(true)

	err := listen.Listen()
	if err != nil {
		panic(err)
	}
}
