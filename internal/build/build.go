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
	// Recheck also runs pair files already stored for a chart when their
	// stageproof_version is not the pinned release, including files the
	// current enumeration does not list.
	Recheck bool
	// Shard and Shards select charts by position in the id-sorted list.
	// Shards 0 means every chart. Shard is the zero-based remainder.
	Shard  int
	Shards int
	// ReleaseJumps, when set, also builds a jump to the newest stable version
	// from each of the newest ReleaseJumps stable versions, and only when that
	// newest version is not already the stored latest. Zero leaves pair
	// selection unchanged.
	ReleaseJumps int
}

// Result counts what the build wrote.
type Result struct {
	Charts  int
	Pairs   int
	Blocked int
	Failed  int
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
	Failed          int    `json:"failed,omitempty"`
	Error           string `json:"error,omitempty"`
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
	list, err := selectCharts(file.Charts, opt)
	if err != nil {
		return Result{}, err
	}
	cat := []catalogChart{}
	var result Result
	for i, chart := range list {
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
		result.Failed += row.Failed
		fmt.Fprintf(os.Stderr, "chart %d/%d %s pairs=%d blocked=%d failed=%d\n", i+1, len(list), chart.ID, row.Pairs, row.Blocked, row.Failed)
		if row.Error != "" {
			fmt.Fprintf(os.Stderr, "  %s\n", row.Error)
		}
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
	row := catalogChart{
		ID: chart.ID, Repo: chart.Repo, RepoName: chart.RepoName, Chart: chart.Chart,
		Stars: chart.Stars, Homepage: chart.Homepage, ReleaseNotesURL: chart.ReleaseNotesURL,
	}
	versions, err := opt.Source.Versions(ctx, chart)
	if err != nil {
		row.Failed = 1
		row.Error = err.Error()
		// A repository that cannot be listed still has its older reports.
		// Leave those files in the catalog instead of publishing a zero.
		dir := filepath.Join(opt.OutDir, "v1", "charts", chart.RepoName, chart.Chart)
		pairs, blocked, countErr := countPairFiles(dir)
		if countErr != nil {
			return catalogChart{}, countErr
		}
		row.Pairs = pairs
		row.Blocked = blocked
		return row, nil
	}
	dir := filepath.Join(opt.OutDir, "v1", "charts", chart.RepoName, chart.Chart)
	chosen := choosePairs(versions, now, opt.MaxPairs, opt.ReleaseJumps, storedLatest(dir))
	if opt.Recheck {
		extra, err := staleStoredPairs(dir, release.Version)
		if err != nil {
			return catalogChart{}, err
		}
		chosen = mergePairs(chosen, extra)
	}
	row.Pairs = len(chosen)
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
	if err := writeFile(filepath.Join(dir, "meta.json"), buf); err != nil {
		return catalogChart{}, err
	}
	for _, pair := range chosen {
		if _, err := buildPair(ctx, chart, pair, opt, minors, dir); err != nil {
			row.Failed++
			if row.Error == "" {
				row.Error = err.Error()
			}
		}
	}
	// Files already on disk stay in the catalog. A nightly run that checks
	// only the newest pair must not drop the older reports from the counts.
	pairs, blocked, err := countPairFiles(dir)
	if err != nil {
		return catalogChart{}, err
	}
	row.Pairs = pairs
	row.Blocked = blocked
	return row, nil
}

func choosePairs(versions []pairs.Version, now time.Time, maxPairs, releaseJumps int, stored string) []pairs.Pair {
	chosen := pairs.Enumerate(versions, now)
	if maxPairs > 0 && len(chosen) > maxPairs {
		chosen = chosen[len(chosen)-maxPairs:]
	}
	if releaseJumps <= 0 || !pairs.LatestChanged(versions, stored) {
		return chosen
	}
	return mergePairs(chosen, pairs.ToLatest(versions, releaseJumps))
}

func storedLatest(dir string) string {
	buf, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return ""
	}
	var doc struct {
		Latest string `json:"latest"`
	}
	if err := json.Unmarshal(buf, &doc); err != nil {
		return ""
	}
	return doc.Latest
}

func countPairFiles(dir string) (int, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	pairs, blocked := 0, 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == "meta.json" || !strings.HasSuffix(name, ".json") {
			continue
		}
		buf, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return 0, 0, err
		}
		var doc struct {
			Results map[string]any `json:"k8s_results"`
		}
		dec := json.NewDecoder(bytes.NewReader(buf))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			return 0, 0, fmt.Errorf("%s: %w", name, err)
		}
		pairs++
		for _, minor := range report.MinorKeys(doc.Results) {
			if report.Verdict(doc.Results[minor]) == "BLOCKED" {
				blocked++
				break
			}
		}
	}
	return pairs, blocked, nil
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
		reproduceRepo(chart.Repo, chart.Chart), chart.Chart, pair.From, pair.To, minor, api,
	)
}

// reproduceRepo strips the trailing /<chart> segment from full OCI chart URLs
// (such as Artifact Hub entries) so stageproof check --repo <url> --chart <name>
// resolves <url>/<name> without duplicating the chart name.
func reproduceRepo(repo, chart string) string {
	if !strings.HasPrefix(repo, "oci://") || chart == "" {
		return repo
	}
	trimmed := strings.TrimRight(repo, "/")
	suffix := "/" + chart
	if !strings.HasSuffix(trimmed, suffix) {
		return repo
	}
	parent := strings.TrimSuffix(trimmed, suffix)
	if !strings.Contains(strings.TrimPrefix(parent, "oci://"), "/") {
		return repo
	}
	return parent
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

// ParseShard reads "i/n". An empty string means every chart.
func ParseShard(raw string) (shard, shards int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, nil
	}
	left, right, ok := strings.Cut(raw, "/")
	if !ok {
		return 0, 0, fmt.Errorf("shard %q must look like 3/40", raw)
	}
	shard, err = atoi(left)
	if err != nil {
		return 0, 0, fmt.Errorf("shard %q must look like 3/40", raw)
	}
	shards, err = atoi(right)
	if err != nil || shards < 1 || shard >= shards {
		return 0, 0, fmt.Errorf("shard %q is out of range", raw)
	}
	return shard, shards, nil
}

func atoi(raw string) (int, error) {
	n := 0
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func selectCharts(list []charts.Chart, opt Options) ([]charts.Chart, error) {
	if opt.Shards == 0 {
		if opt.Limit > 0 && opt.Limit < len(list) {
			return list[:opt.Limit], nil
		}
		return list, nil
	}
	if opt.Shard < 0 || opt.Shard >= opt.Shards {
		return nil, fmt.Errorf("shard %d/%d is out of range", opt.Shard, opt.Shards)
	}
	sorted := append([]charts.Chart(nil), list...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var out []charts.Chart
	for i, chart := range sorted {
		if i%opt.Shards == opt.Shard {
			out = append(out, chart)
		}
	}
	if opt.Limit > 0 && opt.Limit < len(out) {
		out = out[:opt.Limit]
	}
	return out, nil
}

// ShardSlugs returns repo_name/chart for one shard, in id order.
func ShardSlugs(list []charts.Chart, shard, shards int) ([]string, error) {
	chosen, err := selectCharts(list, Options{Shard: shard, Shards: shards})
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(chosen))
	for _, chart := range chosen {
		slugs = append(slugs, chart.RepoName+"/"+chart.Chart)
	}
	return slugs, nil
}

func staleStoredPairs(dir, version string) ([]pairs.Pair, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	var out []pairs.Pair
	for _, name := range names {
		pair, ok := pairFromFile(name)
		if !ok {
			continue
		}
		got, readable := storedVersion(filepath.Join(dir, name))
		if readable && got == version {
			continue
		}
		out = append(out, pair)
	}
	return out, nil
}

func pairFromFile(name string) (pairs.Pair, bool) {
	if name == "meta.json" || !strings.HasSuffix(name, ".json") {
		return pairs.Pair{}, false
	}
	stem := strings.TrimSuffix(name, ".json")
	from, to, ok := strings.Cut(stem, "..")
	if !ok || from == "" || to == "" || strings.Contains(from, "..") || strings.Contains(to, "..") {
		return pairs.Pair{}, false
	}
	if strings.Contains(from, "/") || strings.Contains(to, "/") {
		return pairs.Pair{}, false
	}
	return pairs.Pair{From: from, To: to}, true
}

func storedVersion(path string) (string, bool) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var doc struct {
		Version string `json:"stageproof_version"`
	}
	if err := json.Unmarshal(buf, &doc); err != nil {
		return "", false
	}
	return doc.Version, true
}

func mergePairs(chosen, extra []pairs.Pair) []pairs.Pair {
	seen := map[string]bool{}
	for _, pair := range chosen {
		seen[pair.From+"\x00"+pair.To] = true
	}
	for _, pair := range extra {
		key := pair.From + "\x00" + pair.To
		if seen[key] {
			continue
		}
		seen[key] = true
		chosen = append(chosen, pair)
	}
	return chosen
}

// WriteCatalog rewrites data/v1/index.json from the pair files already on disk.
// It does not run Stageproof.
func WriteCatalog(file charts.File, outDir string) error {
	cat := make([]catalogChart, 0, len(file.Charts))
	for _, chart := range file.Charts {
		if err := chart.Validate(); err != nil {
			return err
		}
		dir := filepath.Join(outDir, "v1", "charts", chart.RepoName, chart.Chart)
		n, blocked, err := countPairFiles(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", chart.ID, err)
		}
		row := catalogChart{
			ID: chart.ID, Repo: chart.Repo, RepoName: chart.RepoName, Chart: chart.Chart,
			Stars: chart.Stars, Homepage: chart.Homepage, ReleaseNotesURL: chart.ReleaseNotesURL,
			Latest: readLatest(dir), Pairs: n, Blocked: blocked,
		}
		cat = append(cat, row)
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
		return err
	}
	return writeFile(filepath.Join(outDir, "v1", "index.json"), buf)
}

func readLatest(dir string) string {
	buf, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return ""
	}
	var doc struct {
		Latest string `json:"latest"`
	}
	if err := json.Unmarshal(buf, &doc); err != nil {
		return ""
	}
	return doc.Latest
}
