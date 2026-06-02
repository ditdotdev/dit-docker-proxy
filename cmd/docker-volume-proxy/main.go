/*
 * Copyright Dit.
 */

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ditdotdev/dit-docker-proxy/internal/forwarder"
	"github.com/ditdotdev/dit-docker-proxy/internal/listener"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

// run holds the body of main so it's exercised by tests. It owns its own
// FlagSet (rather than the global flag.CommandLine) so tests can call it
// with arbitrary args without leaking state between cases.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("docker-volume-forwarder", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: docker-volume-forwarder [--host host] [--port port] socket\n")
		fs.PrintDefaults()
	}

	host := fs.String("host", "localhost", "host to connect to")
	port := fs.Int("port", 5001, "port to connect to")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return fmt.Errorf("missing required socket path")
	}
	path := fs.Arg(0)

	_, _ = fmt.Fprintf(stdout, "Proxying requests from %s to %s:%d\n", path, *host, *port)

	forward := forwarder.New(*host, *port)
	listen := listener.New(forward, path)
	listen.SetLogging(true)
	return listen.Listen()
}
