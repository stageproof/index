package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stageproof/index/internal/census"
)

func main() {
	data := flag.String("data", "data", "directory that contains v1/charts")
	out := flag.String("o", "", "write the report to this file instead of stdout")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "report: unexpected arguments")
		os.Exit(2)
	}
	sum, err := census.Read(*data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "report: %v\n", err)
		os.Exit(1)
	}
	text := census.Markdown(sum)
	if *out == "" {
		fmt.Print(text)
		return
	}
	if err := os.WriteFile(*out, []byte(text), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "report: %v\n", err)
		os.Exit(1)
	}
}
