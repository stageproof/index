// Package build runs the pinned Stageproof release over chart pairs and writes the index.
package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/pairs"
	"github.com/stageproof/index/internal/release"
	"github.com/stageproof/index/internal/report"
	"github.com/stageproof/index/internal/source"
)

const notice = "chart defaults only; run with your values files"

// Options is one index build.
type Options struct {
	OutDir       string
	Binary       string
	Minors       []string
	APIServer    string
	APIServerDir string
	Limit        int
	MaxPairs     int
	Now          time.Time
	Source       source.Source
}

// Result counts what the build wrote.
type Result struct {
	Charts  int
	Pairs   int
	Blocked int
}

type catalogChart struct {
	ID              string `json:"id"`
	Repo            string `json:"repo"`
	RepoName        string `json:"repo_name"`
	Chart           string `json:"chart"`
	Stars           int    `json:"stars"`
	Homepage        string `json:"homepage"`
	ReleaseNotesURL string `json:"release_notes_url"`
	Latest          string `json:"latest"`
	Pairs           int    `json:"pairs"`
	Blocked         int    `json:"blocked"`
}

// Run writes data/v1 under OutDir.
func Run(ctx context.Context, file charts.File, opt Options) (Result, error) {
	if opt.Source == nil {
		return Result{}, fmt.Errorf("chart source is required")
	}
	if opt.Binary == "" {
		return Result{}, fmt.Errorf("stageproof binary is required")
	}
	if opt.APIServer == "" {
		opt.APIServer = "auto"
	}
	if len(opt.Minors) == 0 {
		opt.Minors = []string{"1.34", "1.35", "1.36", "1.37"}
	}
	minors := append([]string(nil), opt.Minors...)
	sort.Strings(minors)
	now := opt.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	list := file.Charts
	if opt.Limit > 0 && opt.Limit < len(list) {
		list = list[:opt.Limit]
	}
	cat := []catalogChart{}
	var result Result
	for _, chart := range list {
		if err := chart.Validate(); err != nil {
			return result, err
		}
		row, err := buildChart(ctx, chart, opt, minors, now)
		if err != nil {
			return result, fmt.Errorf("%s: %w", chart.ID, err)
		}
		cat = append(cat, row)
		result.Charts++
		result.Pairs += row.Pairs
		result.Blocked += row.Blocked
	}
	sort.Slice(cat, func(i, j int) bool { return cat[i].ID < cat[j].ID })
	body := map[string]any{
		"schema_version":     1,
		"stageproof_version": release.Version,
		"notice":             notice,
		"charts":             cat,
	}
	buf, err := report.Marshal(body)
	if err != nil {
		return result, err
	}
	if err := writeFile(filepath.Join(opt.OutDir, "v1", "index.json"), buf); err != nil {
		return result, err
	}
	return result, nil
}

func buildChart(ctx context.Context, chart charts.Chart, opt Options, minors []string, now time.Time) (catalogChart, error) {
	versions, err := opt.Source.Versions(ctx, chart)
	if err != nil {
		return catalogChart{}, err
	}
	chosen := pairs.Enumerate(versions, now)
	if opt.MaxPairs > 0 && len(chosen) > opt.MaxPairs {
		chosen = chosen[len(chosen)-opt.MaxPairs:]
	}
	row := catalogChart{
		ID: chart.ID, Repo: chart.Repo, RepoName: chart.RepoName, Chart: chart.Chart,
		Stars: chart.Stars, Homepage: chart.Homepage, ReleaseNotesURL: chart.ReleaseNotesURL,
		Pairs: len(chosen),
	}
	if stable := pairs.Stable(versions); len(stable) > 0 {
		row.Latest = stable[len(stable)-1].Name
	}
	meta := map[string]any{
		"id": chart.ID, "repo": chart.Repo, "repo_name": chart.RepoName, "chart": chart.Chart,
		"stars": chart.Stars, "homepage": chart.Homepage, "release_notes_url": chart.ReleaseNotesURL,
		"latest": row.Latest, "versions": stableNames(versions),
	}
	buf, err := report.Marshal(meta)
	if err != nil {
		return catalogChart{}, err
	}
	dir := filepath.Join(opt.OutDir, "v1", "charts", chart.RepoName, chart.Chart)
	if err := writeFile(filepath.Join(dir, "meta.json"), buf); err != nil {
		return catalogChart{}, err
	}
	for _, pair := range chosen {
		blocked, err := buildPair(ctx, chart, pair, opt, minors, dir)
		if err != nil {
			return catalogChart{}, err
		}
		if blocked {
			row.Blocked++
		}
	}
	return row, nil
}

func buildPair(ctx context.Context, chart charts.Chart, pair pairs.Pair, opt Options, minors []string, dir string) (blocked bool, err error) {
	if strings.Contains(pair.From, "..") || strings.Contains(pair.To, "..") || strings.Contains(pair.From, "/") || strings.Contains(pair.To, "/") {
		return false, fmt.Errorf("version %s..%s is not a safe file name", pair.From, pair.To)
	}
	fromDir, fromSum, cleanFrom, err := opt.Source.Fetch(ctx, chart, pair.From)
	if err != nil {
		return false, err
	}
	defer func() {
		if cerr := cleanFrom(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	toDir, toSum, cleanTo, err := opt.Source.Fetch(ctx, chart, pair.To)
	if err != nil {
		return false, err
	}
	defer func() {
		if cerr := cleanTo(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	name := filepath.Join(dir, pair.From+".."+pair.To+".json")
	if blocked, ok := reuse(name, fromSum, toSum, minors); ok {
		return blocked, nil
	}
	results := map[string]any{}
	for _, minor := range minors {
		raw, err := runStageproof(ctx, opt, fromDir, toDir, minor)
		if err != nil {
			return false, fmt.Errorf("%s -> %s on %s: %w", pair.From, pair.To, minor, err)
		}
		normalized, err := report.Normalize(raw)
		if err != nil {
			return false, err
		}
		scrub(normalized, fromDir, pair.From, toDir, pair.To)
		results[minor] = normalized
		if report.Verdict(normalized) == "BLOCKED" {
			blocked = true
		}
	}
	primary := minors[0]
	body := map[string]any{
		"schema_version":     1,
		"stageproof_version": release.Version,
		"scope":              "chart-defaults-only",
		"notice":             notice,
		"reproduce_command":  reproduce(chart, pair, primary, opt.APIServer),
		"from_sha256":        fromSum,
		"to_sha256":          toSum,
		"k8s_results":        results,
	}
	buf, err := report.Marshal(body)
	if err != nil {
		return false, err
	}
	if err := writeFile(name, buf); err != nil {
		return false, err
	}
	return blocked, nil
}

func reuse(path, fromSum, toSum string, minors []string) (blocked bool, ok bool) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return false, false
	}
	if doc["stageproof_version"] != release.Version || doc["from_sha256"] != fromSum || doc["to_sha256"] != toSum {
		return false, false
	}
	results, _ := doc["k8s_results"].(map[string]any)
	for _, minor := range minors {
		one, found := results[minor]
		if !found {
			return false, false
		}
		if report.Verdict(one) == "BLOCKED" {
			blocked = true
		}
	}
	return blocked, true
}

func reproduce(chart charts.Chart, pair pairs.Pair, minor, api string) string {
	return fmt.Sprintf(
		"stageproof check --repo %s --chart %s --from %s --to %s --values-proof full --current-k8s %s --api-server %s --format json",
		chart.Repo, chart.Chart, pair.From, pair.To, minor, api,
	)
}

func runStageproof(ctx context.Context, opt Options, fromDir, toDir, minor string) ([]byte, error) {
	args := []string{
		"check",
		"--current-chart", fromDir,
		"--new-chart", toDir,
		"--values-proof", "full",
		"--current-k8s", minor,
		"--api-server", opt.APIServer,
		"--format", "json",
	}
	if opt.APIServerDir != "" {
		args = append(args, "--api-server-dir", opt.APIServerDir)
	}
	cmd := exec.CommandContext(ctx, opt.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if stdout.Len() == 0 {
		msg := strings.TrimSpace(stderr.String())
		if err != nil {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, fmt.Errorf("stageproof wrote no report: %s", msg)
	}
	if !json.Valid(stdout.Bytes()) {
		return nil, fmt.Errorf("stageproof wrote non-json (exit %v): %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func scrub(v any, fromDir, fromVer, toDir, toVer string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok {
				t[k] = strings.ReplaceAll(strings.ReplaceAll(s, fromDir, fromVer), toDir, toVer)
				continue
			}
			scrub(val, fromDir, fromVer, toDir, toVer)
		}
	case []any:
		for i, val := range t {
			if s, ok := val.(string); ok {
				t[i] = strings.ReplaceAll(strings.ReplaceAll(s, fromDir, fromVer), toDir, toVer)
				continue
			}
			scrub(val, fromDir, fromVer, toDir, toVer)
		}
	}
}

func stableNames(versions []pairs.Version) []string {
	stable := pairs.Stable(versions)
	names := make([]string, 0, len(stable))
	for _, v := range stable {
		names = append(names, v.Name)
	}
	return names
}

func writeFile(path string, buf []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}
