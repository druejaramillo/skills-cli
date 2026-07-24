package main

import (
	"context"
	"fmt"
	"os"

	"github.com/druejaramillo/skills-cli/internal/cli"
)

func main() {
	app, err := cli.New()
	if err == nil {
		err = app.Run(context.Background(), os.Args[1:])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "skills: %v\n", err)
		os.Exit(1)
	}
}
