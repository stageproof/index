package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stageproof/index/internal/build"
	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/release"
	"github.com/stageproof/index/internal/source"
)

func main() {
	list := flag.String("charts", "charts.yaml", "chart list")
	out := flag.String("o", "data", "output directory")
	bin := flag.String("binary", "", "pinned stageproof binary; downloaded when empty")
	minors := flag.String("minors", "1.34,1.35,1.36,1.37", "comma-separated Kubernetes minors")
	api := flag.String("api-server", "auto", "stageproof --api-server value")
	apiDir := flag.String("api-server-dir", "", "stageproof --api-server-dir value")
	limit := flag.Int("limit", 0, "build at most this many charts; 0 builds the whole list")
	maxPairs := flag.Int("max-pairs", 0, "build at most this many pairs per chart; 0 builds every pair")
	releaseJumps := flag.Int("release-jumps", 0, "when the newest version changed, also jump to it from each of the newest N stable versions")
	recheck := flag.Bool("recheck", false, "also re-check stored pair files whose stageproof_version is not the pin")
	shardFlag := flag.String("shard", "", "build charts i/n of the id-sorted list, for example 3/40")
	listShard := flag.String("list-shard", "", "print repo_name/chart for shard i/n and exit")
	catalog := flag.Bool("catalog", false, "rewrite data/v1/index.json from pair files on disk and exit")
	flag.Parse()

	file, err := charts.Load(*list)
	if err != nil {
		fail(err)
	}
	if *listShard != "" {
		shard, shards, err := build.ParseShard(*listShard)
		if err != nil {
			fail(err)
		}
		slugs, err := build.ShardSlugs(file.Charts, shard, shards)
		if err != nil {
			fail(err)
		}
		for _, slug := range slugs {
			fmt.Println(slug)
		}
		return
	}
	if *catalog {
		if err := build.WriteCatalog(file, *out); err != nil {
			fail(err)
		}
		fmt.Println("catalog rewritten")
		return
	}
	shard, shards, err := build.ParseShard(*shardFlag)
	if err != nil {
		fail(err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		fail(err)
	}
	binary := *bin
	if binary == "" {
		binary, err = release.Ensure(nil, "", filepath.Join(cache, "stageproof-index", release.Version))
		if err != nil {
			fail(err)
		}
	}
	res, err := build.Run(context.Background(), file, build.Options{
		OutDir:       *out,
		Binary:       binary,
		Minors:       split(*minors),
		APIServer:    *api,
		APIServerDir: *apiDir,
		Limit:        *limit,
		MaxPairs:     *maxPairs,
		ReleaseJumps: *releaseJumps,
		Recheck:      *recheck,
		Shard:        shard,
		Shards:       shards,
		Source:       &source.HTTP{CacheDir: filepath.Join(cache, "stageproof-index", "charts")},
	})
	if err != nil {
		fail(err)
	}
	fmt.Printf("charts %d pairs %d blocked %d failed %d\n", res.Charts, res.Pairs, res.Blocked, res.Failed)
}

func split(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "build: %v\n", err)
	os.Exit(1)
}
