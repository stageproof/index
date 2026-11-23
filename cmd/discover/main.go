package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/discover"
)

func main() {
	n := flag.Int("n", 300, "how many charts to keep")
	out := flag.String("o", "charts.yaml", "where to write the chart list")
	flag.Parse()
	file, err := discover.Run(discover.Options{Limit: *n})
	if err != nil {
		fmt.Fprintf(os.Stderr, "discover: %v\n", err)
		os.Exit(1)
	}
	if err := charts.Write(*out, file); err != nil {
		fmt.Fprintf(os.Stderr, "discover: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d charts to %s\n", len(file.Charts), *out)
}
