// Command prodshadow rehearses PostgreSQL migrations under concurrent load in
// disposable databases and reports measured evidence for CI.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/BhuwanJoshi-01/prodshadow/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	dir, _ := os.Getwd()
	code := cli.Main(ctx, os.Args[1:], &cli.Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.LookupEnv, Dir: dir})
	stop()
	os.Exit(code)
}
