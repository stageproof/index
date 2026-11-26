// Package census counts the committed index reports.
// A pair with any BLOCKED Kubernetes minor is blocked. Otherwise a pair with
// any NOT CHECKED minor is not checked. Every other pair is passed.
// That is the same rule the site uses.
package census

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

//go:embed accuracy.json
var accuracyJSON []byte

// Summary is one pass over data/v1/charts and the catalog in data/v1/index.json.
type Summary struct {
	Catalog      int
	Charts       int
	WithoutPairs []CatalogGap
	Pairs        int
	Blocked      int
	NotChecked   int
	Passed       int
	Minors       []string
	Steps        StepCounts
	Causes       CauseCounts
	Versions     []Count
	Rules        []Rule
	Accuracy     Accuracy
	Orphans      []OrphanVersion
}

// OrphanVersion is a chart version named by stored pairs and absent from
// that chart's meta.json versions list. The upstream index dropped or
// renamed the string. The files are kept.
type OrphanVersion struct {
	ID      string
	Version string
	Pairs   int
}

// StepCounts is the newest adjacent upgrade of each chart that has a stored pair.
// None is a chart whose stored pairs all skip a version.
type StepCounts struct {
	Blocked    int
	NotChecked int
	Passed     int
	None       int
}

// CauseCounts splits blocked pairs by the blocker rules in the file.
type CauseCounts struct {
	Removed int
	Render  int
	Hazard  int
	Mixed   int
}

// Accuracy is the copied corpus measurement. The numbers live in accuracy.json.
type Accuracy struct {
	Source     string   `json:"source"`
	Repository string   `json:"repository"`
	Commit     string   `json:"commit"`
	Date       string   `json:"date"`
	Workflow   string   `json:"workflow_run"`
	Runner     string   `json:"runner"`
	Lines      []string `json:"lines"`
	Reproduce  []string `json:"reproduce"`
}

// CatalogGap is a catalog chart with no stored upgrade file.
type CatalogGap struct {
	ID    string
	Error string
}

// Count is a named total.
type Count struct {
	Name  string
	Count int
}

// Rule is one blocker rule across the index.
// Pairs is the number of upgrade files that contain it.
// Findings is the number of blocker findings, which can be larger.
type Rule struct {
	ID       string
	Pairs    int
	Findings int
	Example  Example
}

// Example is the stored pair with the newest target version for a rule.
type Example struct {
	Repo    string
	Chart   string
	From    string
	To      string
	Version string
	Message string
	Command string
}

type envelope struct {
	StageproofVersion string               `json:"stageproof_version"`
	ReproduceCommand  string               `json:"reproduce_command"`
	Results           map[string]k8sReport `json:"k8s_results"`
}

type k8sReport struct {
	Verdict  string    `json:"verdict"`
	Findings []finding `json:"findings"`
}

type finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Ignored  bool   `json:"ignored"`
}

// Read walks dataDir/v1/charts and counts pair files.
func Read(dataDir string) (Summary, error) {
	root := filepath.Join(dataDir, "v1", "charts")
	var sum Summary
	versions := map[string]int{}
	rules := map[string]*Rule{}
	charts := map[string]struct{}{}
	minors := map[string]struct{}{}
	versionCache := map[string][]string{}
	newest := map[string]*adjacentPair{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || name == "meta.json" {
			return nil
		}
		stem := strings.TrimSuffix(name, ".json")
		from, to, ok := strings.Cut(stem, "..")
		if !ok || from == "" || to == "" || strings.Contains(to, "..") {
			return fmt.Errorf("%s is not a from..to pair file", path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc envelope
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 3 {
			return fmt.Errorf("%s is not charts/<repo>/<chart>/<from>..<to>.json", path)
		}
		repo, chart := parts[0], parts[1]
		charts[repo+"/"+chart] = struct{}{}
		sum.Pairs++
		for minor := range doc.Results {
			minors[minor] = struct{}{}
		}
		version := doc.StageproofVersion
		if version == "" {
			version = "(none)"
		}
		versions[version]++
		blocked, notChecked := pairVerdict(doc)
		switch {
		case blocked:
			sum.Blocked++
		case notChecked:
			sum.NotChecked++
		default:
			sum.Passed++
		}
		vers, err := chartVersions(filepath.Dir(path), versionCache)
		if err != nil {
			return err
		}
		for _, missing := range absentVersions(vers, from, to) {
			sum.noteOrphan(repo+"/"+chart, missing)
		}
		if adjacent(vers, from, to) {
			id := repo + "/" + chart
			cand := &adjacentPair{From: from, To: to, Blocked: blocked, NotChecked: notChecked}
			if cur := newest[id]; cur == nil || newerPair(cand.From, cand.To, cur.From, cur.To) {
				newest[id] = cand
			}
		}
		if blocked {
			switch causeOf(doc) {
			case "removed":
				sum.Causes.Removed++
			case "render":
				sum.Causes.Render++
			case "mixed":
				sum.Causes.Mixed++
			default:
				sum.Causes.Hazard++
			}
		}
		seen := map[string]bool{}
		for _, minor := range minorKeys(doc.Results) {
			for _, item := range doc.Results[minor].Findings {
				if item.Severity != "blocker" || item.Ignored || item.RuleID == "" {
					continue
				}
				rule := rules[item.RuleID]
				if rule == nil {
					rule = &Rule{ID: item.RuleID}
					rules[item.RuleID] = rule
				}
				rule.Findings++
				if seen[item.RuleID] {
					continue
				}
				seen[item.RuleID] = true
				rule.Pairs++
				example := Example{
					Repo: repo, Chart: chart, From: from, To: to,
					Version: doc.StageproofVersion, Message: item.Message, Command: doc.ReproduceCommand,
				}
				if rule.Example.Command == "" || newerExample(example, rule.Example) {
					rule.Example = example
				}
			}
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	sort.Slice(sum.Orphans, func(i, j int) bool {
		if sum.Orphans[i].ID != sum.Orphans[j].ID {
			return sum.Orphans[i].ID < sum.Orphans[j].ID
		}
		return sum.Orphans[i].Version < sum.Orphans[j].Version
	})
	sum.Charts = len(charts)
	for id := range charts {
		step := newest[id]
		if step == nil {
			sum.Steps.None++
			continue
		}
		switch {
		case step.Blocked:
			sum.Steps.Blocked++
		case step.NotChecked:
			sum.Steps.NotChecked++
		default:
			sum.Steps.Passed++
		}
	}
	for minor := range minors {
		sum.Minors = append(sum.Minors, minor)
	}
	sort.Strings(sum.Minors)
	accuracy, err := loadAccuracy()
	if err != nil {
		return Summary{}, err
	}
	sum.Accuracy = accuracy
	gaps, catalogN, err := catalogGaps(filepath.Join(dataDir, "v1", "index.json"))
	if err != nil {
		return Summary{}, err
	}
	sum.Catalog = catalogN
	sum.WithoutPairs = gaps
	for name, n := range versions {
		sum.Versions = append(sum.Versions, Count{Name: name, Count: n})
	}
	sort.Slice(sum.Versions, func(i, j int) bool {
		if sum.Versions[i].Count != sum.Versions[j].Count {
			return sum.Versions[i].Count > sum.Versions[j].Count
		}
		return sum.Versions[i].Name < sum.Versions[j].Name
	})
	for _, rule := range rules {
		sum.Rules = append(sum.Rules, *rule)
	}
	sort.Slice(sum.Rules, func(i, j int) bool {
		if sum.Rules[i].Pairs != sum.Rules[j].Pairs {
			return sum.Rules[i].Pairs > sum.Rules[j].Pairs
		}
		return sum.Rules[i].ID < sum.Rules[j].ID
	})
	return sum, nil
}

func catalogGaps(path string) ([]CatalogGap, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	var doc struct {
		Charts []struct {
			ID    string `json:"id"`
			Pairs int    `json:"pairs"`
			Error string `json:"error"`
		} `json:"charts"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}
	var gaps []CatalogGap
	for _, chart := range doc.Charts {
		if chart.Pairs > 0 {
			continue
		}
		gaps = append(gaps, CatalogGap{ID: chart.ID, Error: chart.Error})
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].ID < gaps[j].ID })
	return gaps, len(doc.Charts), nil
}

func pathLess(a, b Example) bool {
	if a.Repo != b.Repo {
		return a.Repo < b.Repo
	}
	if a.Chart != b.Chart {
		return a.Chart < b.Chart
	}
	if a.From != b.From {
		return a.From < b.From
	}
	return a.To < b.To
}

func pairVerdict(doc envelope) (blocked, notChecked bool) {
	sawNotChecked := false
	for _, minor := range minorKeys(doc.Results) {
		switch doc.Results[minor].Verdict {
		case "BLOCKED":
			return true, false
		case "NOT CHECKED":
			sawNotChecked = true
		}
	}
	return false, sawNotChecked
}

func (sum *Summary) noteOrphan(id, version string) {
	for i := range sum.Orphans {
		if sum.Orphans[i].ID == id && sum.Orphans[i].Version == version {
			sum.Orphans[i].Pairs++
			return
		}
	}
	sum.Orphans = append(sum.Orphans, OrphanVersion{ID: id, Version: version, Pairs: 1})
}

func absentVersions(versions []string, from, to string) []string {
	if len(versions) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, version := range versions {
		have[version] = true
	}
	var missing []string
	if from != "" && !have[from] {
		missing = append(missing, from)
	}
	if to != "" && to != from && !have[to] {
		missing = append(missing, to)
	}
	sort.Strings(missing)
	return missing
}

func minorKeys(results map[string]k8sReport) []string {
	keys := make([]string, 0, len(results))
	for key := range results {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type adjacentPair struct {
	From       string
	To         string
	Blocked    bool
	NotChecked bool
}

func chartVersions(dir string, cache map[string][]string) ([]string, error) {
	if vers, ok := cache[dir]; ok {
		return vers, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			cache[dir] = nil
			return nil, nil
		}
		return nil, err
	}
	var doc struct {
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, "meta.json"), err)
	}
	cache[dir] = doc.Versions
	return doc.Versions, nil
}

func adjacent(versions []string, from, to string) bool {
	for i := 0; i+1 < len(versions); i++ {
		if versions[i] == from && versions[i+1] == to {
			return true
		}
	}
	return false
}

func newerPair(from, to, otherFrom, otherTo string) bool {
	if c := semver.Compare(canon(to), canon(otherTo)); c != 0 {
		return c > 0
	}
	return semver.Compare(canon(from), canon(otherFrom)) > 0
}

func newerExample(a, b Example) bool {
	if c := semver.Compare(canon(a.To), canon(b.To)); c != 0 {
		return c > 0
	}
	if c := semver.Compare(canon(a.From), canon(b.From)); c != 0 {
		return c > 0
	}
	return pathLess(a, b)
}

func canon(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return ""
	}
	return semver.Canonical(v)
}

func causeOf(doc envelope) string {
	removed, render, hazard := false, false, false
	for _, minor := range minorKeys(doc.Results) {
		if doc.Results[minor].Verdict != "BLOCKED" {
			continue
		}
		for _, item := range doc.Results[minor].Findings {
			if item.Severity != "blocker" || item.Ignored || item.RuleID == "" {
				continue
			}
			switch item.RuleID {
			case "k8s.api.removed":
				removed = true
			case "helm.template.render_failed", "helm.values.new_required_unset", "k8s.apply.invalid":
				render = true
			default:
				hazard = true
			}
		}
	}
	n := 0
	if removed {
		n++
	}
	if render {
		n++
	}
	if hazard {
		n++
	}
	switch {
	case n > 1:
		return "mixed"
	case removed:
		return "removed"
	case render:
		return "render"
	default:
		return "hazard"
	}
}

func loadAccuracy() (Accuracy, error) {
	var doc Accuracy
	if err := json.Unmarshal(accuracyJSON, &doc); err != nil {
		return Accuracy{}, fmt.Errorf("accuracy.json: %w", err)
	}
	if doc.Commit == "" || doc.Source == "" || len(doc.Lines) == 0 {
		return Accuracy{}, fmt.Errorf("accuracy.json is missing its source, commit, or lines")
	}
	return doc, nil
}

func minorPhrase(minors []string) string {
	if len(minors) == 0 {
		return "The stored files do not name a Kubernetes minor."
	}
	if len(minors) == 1 {
		return "The stored files were checked against Kubernetes " + minors[0] + ". A removed API is a finding about that minor."
	}
	return "The stored files name Kubernetes minors " + strings.Join(minors, ", ") + ". A removed API is a finding about the minor in that file."
}

// Markdown writes the index counts. It does not call the reports safe.
func Markdown(sum Summary) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Stageproof index report\n\n")
	fmt.Fprintf(&buf, "These numbers are chart defaults only. They are not a check of any cluster's values files. Regenerate with `go run ./cmd/report -data data`.\n\n")
	fmt.Fprintf(&buf, "%s\n\n", minorPhrase(sum.Minors))
	fmt.Fprintf(&buf, "Newest adjacent upgrade: %d charts blocked, %d not checked, %d passed, %d with no adjacent step. That is %d charts with a stored pair.\n\n", sum.Steps.Blocked, sum.Steps.NotChecked, sum.Steps.Passed, sum.Steps.None, sum.Charts)
	if sum.Catalog > 0 {
		fmt.Fprintf(&buf, "%d charts are listed. %d have a stored pair.\n\n", sum.Catalog, sum.Charts)
	} else {
		fmt.Fprintf(&buf, "Charts with a stored pair: %d.\n\n", sum.Charts)
	}
	if len(sum.WithoutPairs) > 0 {
		fmt.Fprintf(&buf, "Catalog charts with no stored pair:\n\n")
		for _, gap := range sum.WithoutPairs {
			if gap.Error == "" {
				fmt.Fprintf(&buf, "- `%s`\n", gap.ID)
				continue
			}
			fmt.Fprintf(&buf, "- `%s`: %s\n", gap.ID, gap.Error)
		}
		fmt.Fprintf(&buf, "\n")
	}
	fmt.Fprintf(&buf, "Upgrade pairs: %d. Blocked: %d. Not checked: %d. Passed: %d.\n\n", sum.Pairs, sum.Blocked, sum.NotChecked, sum.Passed)
	fmt.Fprintf(&buf, "A pair is blocked when any Kubernetes minor in the file is BLOCKED. Otherwise a pair with any NOT CHECKED minor is not checked. The rest are passed.\n\n")
	if len(sum.Orphans) > 0 {
		fmt.Fprintf(&buf, "## Chart versions absent from the upstream index\n\n")
		fmt.Fprintf(&buf, "A stored pair names a version that is not in that chart's `meta.json` versions list. The upstream index dropped or renamed that version string. The file is kept.\n\n")
		for _, item := range sum.Orphans {
			noun := "stored pairs"
			if item.Pairs == 1 {
				noun = "stored pair"
			}
			fmt.Fprintf(&buf, "- `%s` version `%s` is named by %d %s\n", item.ID, item.Version, item.Pairs, noun)
		}
		fmt.Fprintf(&buf, "\n")
	}
	fmt.Fprintf(&buf, "## Why blocked pairs are blocked\n\n")
	fmt.Fprintf(&buf, "A chart-defaults render problem is `helm.template.render_failed`, `helm.values.new_required_unset`, or `k8s.apply.invalid`. An upgrade hazard is any other blocker, including a delete, an immutable field, a CRD change, or a missing image. Mixed means the file has blockers from more than one of those groups.\n\n")
	fmt.Fprintf(&buf, "| Cause | Pairs |\n|---|---:|\n")
	fmt.Fprintf(&buf, "| only a removed API | %d |\n", sum.Causes.Removed)
	fmt.Fprintf(&buf, "| only a chart-defaults render problem | %d |\n", sum.Causes.Render)
	fmt.Fprintf(&buf, "| an upgrade hazard | %d |\n", sum.Causes.Hazard)
	fmt.Fprintf(&buf, "| more than one of those | %d |\n\n", sum.Causes.Mixed)
	fmt.Fprintf(&buf, "## Stageproof version\n\n")
	for _, item := range sum.Versions {
		fmt.Fprintf(&buf, "- `%s`: %d\n", item.Name, item.Count)
	}
	fmt.Fprintf(&buf, "\n## Blocker rules\n\n")
	fmt.Fprintf(&buf, "Pairs is the number of upgrade files that contain the rule. Findings counts each blocker in those files, so one upgrade can add more than one.\n\n")
	fmt.Fprintf(&buf, "| Rule | Pairs | Findings |\n|---|---:|---:|\n")
	for _, rule := range sum.Rules {
		fmt.Fprintf(&buf, "| `%s` | %d | %d |\n", rule.ID, rule.Pairs, rule.Findings)
	}
	fmt.Fprintf(&buf, "\n## One stored command per rule\n\n")
	fmt.Fprintf(&buf, "Each command is the `reproduce_command` stored in that index file. This report copies it. It does not run it again. The pair is the one with the newest target version for that rule. The message is the first blocker finding for that rule in the file.\n\n")
	for _, rule := range sum.Rules {
		ex := rule.Example
		fmt.Fprintf(&buf, "### `%s`\n\n", rule.ID)
		fmt.Fprintf(&buf, "`%s/%s` %s to %s, report `%s`.\n\n", ex.Repo, ex.Chart, ex.From, ex.To, ex.Version)
		fmt.Fprintf(&buf, "%s\n\n", ex.Message)
		fmt.Fprintf(&buf, "```\n%s\n```\n\n", ex.Command)
	}
	acc := sum.Accuracy
	fmt.Fprintf(&buf, "## Accuracy corpus\n\n")
	fmt.Fprintf(&buf, "This command does not re-run the accuracy corpus. The lines below are copied from `%s` in `%s`, dated %s, commit `%s`, workflow `%s`, on %s.\n\n", acc.Source, acc.Repository, acc.Date, acc.Commit, acc.Workflow, acc.Runner)
	for _, line := range acc.Lines {
		fmt.Fprintf(&buf, "- %s\n", line)
	}
	fmt.Fprintf(&buf, "\nReproduce that measurement from the Stageproof repository:\n\n")
	fmt.Fprintf(&buf, "```\n")
	for _, line := range acc.Reproduce {
		fmt.Fprintf(&buf, "%s\n", line)
	}
	fmt.Fprintf(&buf, "```\n")
	return buf.String()
}
