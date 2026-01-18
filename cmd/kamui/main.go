// Package main is the entry point for the Kamui CLI.
// Kamui CLI provides command-line access to the Kamui Platform,
// a PaaS service for deploying and managing applications.
package main

import (
	"os"

	"github.com/kamui-project/kamui-cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
