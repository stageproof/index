// Package census counts the committed index reports.
// A pair with any BLOCKED Kubernetes minor is blocked. Otherwise a pair with
// any NOT CHECKED minor is not checked. Every other pair is passed.
// That is the same rule the site uses.
package census

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Summary is one pass over data/v1/charts and the catalog in data/v1/index.json.
type Summary struct {
	Catalog      int
	Charts       int
	WithoutPairs []CatalogGap
	Pairs        int
	Blocked      int
	NotChecked   int
	Passed       int
	Versions     []Count
	Rules        []Rule
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

// Example is the first upgrade file, in path order, that recorded a rule.
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
				if rule.Example.Command == "" || pathLess(example, rule.Example) {
					rule.Example = example
				}
			}
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	sum.Charts = len(charts)
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

func minorKeys(results map[string]k8sReport) []string {
	keys := make([]string, 0, len(results))
	for key := range results {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Markdown writes the index counts. It does not call the reports safe.
func Markdown(sum Summary) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Stageproof index report\n\n")
	fmt.Fprintf(&buf, "These numbers are chart defaults only. They are not a check of any cluster's values files. Regenerate with `go run ./cmd/report -data data`.\n\n")
	if sum.Catalog > 0 {
		fmt.Fprintf(&buf, "Catalog charts: %d. Charts with a stored pair: %d. Upgrade pairs: %d. Blocked: %d. Not checked: %d. Passed: %d.\n\n", sum.Catalog, sum.Charts, sum.Pairs, sum.Blocked, sum.NotChecked, sum.Passed)
	} else {
		fmt.Fprintf(&buf, "Charts with a stored pair: %d. Upgrade pairs: %d. Blocked: %d. Not checked: %d. Passed: %d.\n\n", sum.Charts, sum.Pairs, sum.Blocked, sum.NotChecked, sum.Passed)
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
	fmt.Fprintf(&buf, "A pair is blocked when any Kubernetes minor in the file is BLOCKED. Otherwise a pair with any NOT CHECKED minor is not checked. The rest are passed.\n\n")
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
	fmt.Fprintf(&buf, "Each command is the `reproduce_command` stored in that index file. This report copies it. It does not run it again. The message is the first blocker finding for that rule in the file.\n\n")
	for _, rule := range sum.Rules {
		ex := rule.Example
		fmt.Fprintf(&buf, "### `%s`\n\n", rule.ID)
		fmt.Fprintf(&buf, "`%s/%s` %s to %s, report `%s`.\n\n", ex.Repo, ex.Chart, ex.From, ex.To, ex.Version)
		fmt.Fprintf(&buf, "%s\n\n", ex.Message)
		fmt.Fprintf(&buf, "```\n%s\n```\n\n", ex.Command)
	}
	fmt.Fprintf(&buf, "## Accuracy corpus\n\n")
	fmt.Fprintf(&buf, "This command does not re-run the accuracy corpus. The measurement below is from `docs/accuracy.md` in `stageproof/stageproof`, dated 2026-11-22, commit `042579b`, workflow `45065690496`, on `ubuntu-latest` with Go 1.27.4, Helm 3.22.1, and Kubernetes 1.35.0.\n\n")
	fmt.Fprintf(&buf, "- 12 of 12 incidents caught, 12 of 12 controls passed.\n")
	fmt.Fprintf(&buf, "- 316 unique pairs across 105 CNCF projects. 281 were labeled clean and 35 were labeled breaks.\n")
	fmt.Fprintf(&buf, "- Labeled-clean wrong blocks: 0 of 281 with `--api-server off`, and 0 of 281 with `--api-server auto`.\n")
	fmt.Fprintf(&buf, "- File check latency: p50 0.74s, p95 4.13s, max 8.07s. API server proof: p50 5.19s, p95 15.41s.\n")
	fmt.Fprintf(&buf, "- Flux replay: 70 of 70 across 5 public repositories, 62 passed, 8 blocked, 0 not checked, 0 errors. p50 3.53s, p95 25.04s.\n\n")
	fmt.Fprintf(&buf, "Reproduce that measurement from the Stageproof repository:\n\n")
	fmt.Fprintf(&buf, "```\n")
	fmt.Fprintf(&buf, "git checkout 042579b\n")
	fmt.Fprintf(&buf, "go run ./cmd/corpus run --set incidents --out incidents.json\n")
	fmt.Fprintf(&buf, "go run ./cmd/corpus run --set pairs --api-server off --out pairs-files.json\n")
	fmt.Fprintf(&buf, "go run ./cmd/corpus run --set pairs --api-server auto --out pairs.json\n")
	fmt.Fprintf(&buf, "go run ./cmd/corpus compare incidents.json corpus/baseline.json\n")
	fmt.Fprintf(&buf, "go run ./cmd/corpus compare pairs-files.json corpus/baseline.json\n")
	fmt.Fprintf(&buf, "go run ./cmd/corpus compare pairs.json corpus/baseline.json\n")
	fmt.Fprintf(&buf, "go run ./cmd/corpus run --set replay --out replay.json\n")
	fmt.Fprintf(&buf, "```\n")
	return buf.String()
}
