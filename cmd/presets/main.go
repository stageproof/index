package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/presets"
)

func main() {
	path := flag.String("charts", "charts.yaml", "curated chart list")
	out := flag.String("o", "", "write the Renovate preset to this file instead of stdout")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "presets: unexpected arguments")
		os.Exit(2)
	}
	file, err := charts.Load(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "presets: %v\n", err)
		os.Exit(1)
	}
	buf, err := presets.Render(file.Charts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "presets: %v\n", err)
		os.Exit(1)
	}
	if *out == "" {
		if _, err := os.Stdout.Write(buf); err != nil {
			fmt.Fprintf(os.Stderr, "presets: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(*out, buf, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "presets: %v\n", err)
		os.Exit(1)
	}
}
