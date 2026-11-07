package main

import (
	"context"
	"os"

	"github.com/IvanSaratov/micommed-reader/internal/cli"
)

func main() { os.Exit(cli.Run(context.Background(), os.Args, os.Stdout, os.Stderr)) }
