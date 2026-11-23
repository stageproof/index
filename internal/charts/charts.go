// Package charts is the curated list the index builds.
package charts

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Chart is one Helm chart the index is allowed to build.
type Chart struct {
	ID              string `yaml:"id" json:"id"`
	Repo            string `yaml:"repo" json:"repo"`
	RepoName        string `yaml:"repo_name" json:"repo_name"`
	Chart           string `yaml:"chart" json:"chart"`
	Stars           int    `yaml:"stars" json:"stars"`
	Homepage        string `yaml:"homepage" json:"homepage"`
	ReleaseNotesURL string `yaml:"release_notes_url" json:"release_notes_url"`
}

// File is the on-disk shape of charts.yaml.
type File struct {
	Charts []Chart `yaml:"charts"`
}

// Load reads a charts.yaml file.
func Load(path string) (File, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var file File
	if err := yaml.Unmarshal(buf, &file); err != nil {
		return File{}, fmt.Errorf("parse %s: %w", path, err)
	}
	for i, chart := range file.Charts {
		if err := chart.Validate(); err != nil {
			return File{}, fmt.Errorf("charts[%d]: %w", i, err)
		}
	}
	return file, nil
}

// Write writes charts.yaml with a trailing newline.
func Write(path string, file File) error {
	buf, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	const header = "# Candidate list from Artifact Hub. Review it before the nightly build.\n"
	return os.WriteFile(path, append([]byte(header), buf...), 0o644)
}

// Validate checks the fields the builder uses as path segments and URLs.
func (c Chart) Validate() error {
	if c.ID == "" || c.Repo == "" || c.RepoName == "" || c.Chart == "" {
		return fmt.Errorf("id, repo, repo_name, and chart are required")
	}
	if strings.Contains(c.ID, "..") || strings.Contains(c.RepoName, "..") || strings.Contains(c.Chart, "..") {
		return fmt.Errorf("path segment must not contain a parent reference")
	}
	if strings.Contains(c.RepoName, "/") || strings.Contains(c.Chart, "/") {
		return fmt.Errorf("repo_name and chart must be a single path segment")
	}
	return nil
}

// Slug is the data/v1/charts directory for this chart: repo_name/chart.
func (c Chart) Slug() string {
	return c.RepoName + "/" + c.Chart
}
