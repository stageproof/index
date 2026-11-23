package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stageproof/index/internal/site"
)

func main() {
	data := flag.String("data", "data", "directory that contains v1/index.json")
	out := flag.String("o", "site", "directory to replace with the static site")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "site: unexpected arguments")
		os.Exit(2)
	}
	if err := site.Generate(*data, *out); err != nil {
		fmt.Fprintf(os.Stderr, "site: %v\n", err)
		os.Exit(1)
	}
}
