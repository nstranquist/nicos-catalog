package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

func main() {
	check := flag.Bool("check", false, "fail when generated files differ")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if err := explorercontract.WriteGenerated(*root, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
