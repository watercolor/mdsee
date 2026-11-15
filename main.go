package main

import (
	"fmt"
	"os"

	"mdsee/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mdsee:", err)
		os.Exit(1)
	}
}
