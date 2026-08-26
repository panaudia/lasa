package main

import (
	_ "embed"
	"fmt"
)

//go:embed example-claims.json
var exampleClaims string

// example prints an annotated claims file. The _comment keys are for the
// reader; strip them before minting (ParseClaims refuses unknown keys on
// purpose, so the file you mint is exactly the payload you get).
func cmdExample(args []string) error {
	if len(args) != 0 {
		return usagef("example takes no arguments")
	}
	fmt.Print(exampleClaims)
	return nil
}
