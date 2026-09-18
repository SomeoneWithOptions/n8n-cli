// Command n8n is a command-line client for the n8n public API.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	code := cli.Run(ctx, os.Args[1:], cli.Options{
		Streams: cli.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
		Version: version.Get(),
	})
	stop()
	os.Exit(code)
}
