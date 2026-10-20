package main

import (
	"fmt"
	"os"

	"rumable/internal/rumable"
)

func main() {
	if err := rumable.Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rumable:", err)
		os.Exit(1)
	}
}
