package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type catalog struct {
	SchemaVersion     int     `json:"schema_version"`
	StageproofVersion string  `json:"stageproof_version"`
	Notice            string  `json:"notice"`
	Charts            []chart `json:"charts"`
}

type chart struct {
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
	Failed          int    `json:"failed"`
	Error           string `json:"error"`
}

type meta struct {
	Versions []string `json:"versions"`
}

type envelope struct {
	StageproofVersion string               `json:"stageproof_version"`
	Scope             string               `json:"scope"`
	Notice            string               `json:"notice"`
	ReproduceCommand  string               `json:"reproduce_command"`
	Results           map[string]k8sReport `json:"k8s_results"`
}

type k8sReport struct {
	Verdict  string    `json:"verdict"`
	ExitCode int       `json:"exit_code"`
	Findings []finding `json:"findings"`
}

type finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Ignored  bool   `json:"ignored"`
}

type pairFile struct {
	From     string
	To       string
	Raw      []byte
	Doc      envelope
	Adjacent bool
	Blocked  bool
}

func loadCatalog(path string) (catalog, error) {
	var cat catalog
	buf, err := os.ReadFile(path)
	if err != nil {
		return cat, err
	}
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	if err := dec.Decode(&cat); err != nil {
		return cat, fmt.Errorf("index.json: %w", err)
	}
	sort.Slice(cat.Charts, func(i, j int) bool { return cat.Charts[i].ID < cat.Charts[j].ID })
	return cat, nil
}

func loadPairs(dir string, versions []string) ([]pairFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []pairFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || name == "meta.json" {
			continue
		}
		stem := strings.TrimSuffix(name, ".json")
		from, to, ok := strings.Cut(stem, "..")
		if !ok || strings.Contains(to, "..") {
			return nil, fmt.Errorf("pair file %s is not from..to", name)
		}
		if err := segment(from); err != nil {
			return nil, err
		}
		if err := segment(to); err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var doc envelope
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		item := pairFile{
			From: from, To: to, Raw: raw, Doc: doc,
			Adjacent: adjacent(versions, from, to),
			Blocked:  blocked(doc),
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Blocked != out[j].Blocked {
			return out[i].Blocked
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out, nil
}

func adjacent(versions []string, from, to string) bool {
	for i := 0; i+1 < len(versions); i++ {
		if versions[i] == from && versions[i+1] == to {
			return true
		}
	}
	return false
}

func blocked(doc envelope) bool {
	for _, minor := range minorKeys(doc.Results) {
		if doc.Results[minor].Verdict == "BLOCKED" {
			return true
		}
	}
	return false
}

func minorKeys(results map[string]k8sReport) []string {
	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
