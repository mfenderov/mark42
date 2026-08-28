package main

import (
	"os"

	"github.com/mfenderov/mark42/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
