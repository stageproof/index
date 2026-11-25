// Package presets writes the Renovate preset that attaches the Stageproof badge.
package presets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/site"
)

const (
	schema = "https://docs.renovatebot.com/renovate-schema.json"
	// Badge image. The registry value stays empty when no rule set stageproofRegistry.
	badge = "![stageproof](https://stageproof-index-badges.stageproof.workers.dev/badge.svg?chart={{encodeURIComponent depName}}&from={{encodeURIComponent currentVersion}}&registry={{#if prBodyDefinitions.stageproofRegistry}}{{encodeURIComponent prBodyDefinitions.stageproofRegistry}}{{/if}}&to={{encodeURIComponent newVersion}})"
)

var managers = []string{"helmv3", "argocd", "flux"}

// File is a Renovate preset.
type File struct {
	Schema       string `json:"$schema"`
	Description  string `json:"description"`
	PackageRules []Rule `json:"packageRules"`
}

// Rule is one packageRule. Empty fields are omitted so each rule keeps the key order of the published preset.
type Rule struct {
	Description       string   `json:"description"`
	MatchManagers     []string `json:"matchManagers"`
	MatchDatasources  []string `json:"matchDatasources"`
	PrBodyNotes       []string `json:"prBodyNotes,omitempty"`
	MatchJsonata      []string `json:"matchJsonata,omitempty"`
	MatchPackageNames []string `json:"matchPackageNames,omitempty"`
	PrBodyDefinitions *Defs    `json:"prBodyDefinitions,omitempty"`
}

// Defs is the registry the badge query reads from prBodyDefinitions.
type Defs struct {
	StageproofRegistry string `json:"stageproofRegistry"`
}

type entry struct {
	name     string
	registry string
}

// Render writes default.json for these catalog charts.
// HTTP rules match the chart name and the repository URL, ignoring a trailing slash.
// A chart name that has more than one HTTP repository names the URL in the rule description.
// OCI rules match Renovate's Docker packageName, which is the catalog URL without oci://.
func Render(list []charts.Chart) ([]byte, error) {
	var httpEntries, ociEntries []entry
	httpCount := map[string]int{}
	for _, chart := range list {
		registry := site.NormRegistry(chart.Repo)
		if registry == "" || chart.Chart == "" {
			return nil, fmt.Errorf("chart %s has no repository or name", chart.ID)
		}
		item := entry{name: chart.Chart, registry: registry}
		if strings.HasPrefix(registry, "oci://") {
			ociEntries = append(ociEntries, item)
			continue
		}
		httpEntries = append(httpEntries, item)
		httpCount[chart.Chart]++
	}
	sortEntries(httpEntries)
	sortEntries(ociEntries)

	rules := []Rule{
		{
			Description:      "Add the Stageproof index badge to HTTP Helm pull requests.",
			MatchManagers:    managers,
			MatchDatasources: []string{"helm"},
			PrBodyNotes:      []string{badge},
		},
		{
			Description:      "Add the Stageproof index badge to OCI Helm pull requests.",
			MatchManagers:    managers,
			MatchDatasources: []string{"docker"},
			PrBodyNotes:      []string{badge},
		},
	}
	for _, item := range httpEntries {
		description := "Repository URL for " + item.name + "."
		if httpCount[item.name] > 1 {
			description = "Repository URL for " + item.name + " at " + item.registry + "."
		}
		rules = append(rules, Rule{
			Description:       description,
			MatchManagers:     managers,
			MatchDatasources:  []string{"helm"},
			MatchJsonata:      []string{httpMatch(item.name, item.registry)},
			PrBodyDefinitions: &Defs{StageproofRegistry: item.registry},
		})
	}
	for _, item := range ociEntries {
		rules = append(rules, Rule{
			Description:       "OCI repository URL for " + item.name + ".",
			MatchManagers:     managers,
			MatchDatasources:  []string{"docker"},
			MatchPackageNames: []string{strings.TrimPrefix(item.registry, "oci://")},
			PrBodyDefinitions: &Defs{StageproofRegistry: item.registry},
		})
	}
	body := File{
		Schema:       schema,
		Description:  "Stageproof index badge for HTTP Helm, Argo CD, and Flux pull requests.",
		PackageRules: rules,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sortEntries(list []entry) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].name != list[j].name {
			return list[i].name < list[j].name
		}
		return list[i].registry < list[j].registry
	})
}

func httpMatch(name, registry string) string {
	return fmt.Sprintf(
		"depName = '%s' and $count(registryUrls[$replace($, /\\/+$/, '') = '%s']) > 0",
		jsonataQuote(name), jsonataQuote(registry),
	)
}

func jsonataQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}
