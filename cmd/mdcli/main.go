package main

import (
	"fmt"
	"os"

	"mdcli/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mdcli:", err)
		os.Exit(1)
	}
}
