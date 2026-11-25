package presets

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stageproof/index/internal/charts"
)

// charts.yaml at index 9ecd6b33, unchanged since d787134. The published
// stageproof/renovate-presets default.json is this exact document.
func TestChartsYAMLMatchesThePublishedPreset(t *testing.T) {
	file, err := charts.Load("../../charts.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Render(file.Charts)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(got)
	const want = "4e3c8759f44989f72d8b3aa21e6f7a31f7a0d7f897243c2c82f514f0c2718891"
	if hex.EncodeToString(sum[:]) != want {
		t.Fatalf("charts.yaml preset hash is %s, want %s", hex.EncodeToString(sum[:]), want)
	}
}

func TestRenderMatchesNameAndRegistry(t *testing.T) {
	got, err := Render([]charts.Chart{
		{ID: "b", Repo: "https://charts.example/b/", RepoName: "b", Chart: "shared"},
		{ID: "a", Repo: "oci://ghcr.io/example/charts/shared", RepoName: "oci", Chart: "shared"},
		{ID: "c", Repo: "https://other.example/charts", RepoName: "c", Chart: "shared"},
		{ID: "d", Repo: "https://solo.example", RepoName: "d", Chart: "solo"},
		{ID: "e", Repo: "oci://registry-1.docker.io/cloudpirates/solo", RepoName: "e", Chart: "solo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{
		`"description": "Add the Stageproof index badge to HTTP Helm pull requests."`,
		`"description": "Add the Stageproof index badge to OCI Helm pull requests."`,
		"Repository URL for shared at https://charts.example/b.",
		"Repository URL for shared at https://other.example/charts.",
		"Repository URL for solo.",
		"OCI repository URL for shared.",
		"OCI repository URL for solo.",
		`depName = 'shared' and $count(registryUrls[$replace($, /\\/+$/, '') = 'https://charts.example/b']) > 0`,
		`"matchPackageNames": [
        "ghcr.io/example/charts/shared"
      ]`,
		`"stageproofRegistry": "oci://registry-1.docker.io/cloudpirates/solo"`,
		"{{#if prBodyDefinitions.stageproofRegistry}}{{encodeURIComponent prBodyDefinitions.stageproofRegistry}}{{/if}}",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s\n%s", want, text)
		}
	}
	if strings.Contains(text, "Repository URL for solo at ") {
		t.Fatal("a single HTTP repository should not repeat the URL in the description")
	}
	if strings.Contains(text, "https://charts.example/b/") {
		t.Fatal("trailing slash survived")
	}
	sharedAt := strings.Index(text, "Repository URL for shared at https://charts.example/b.")
	otherAt := strings.Index(text, "Repository URL for shared at https://other.example/charts.")
	solo := strings.Index(text, "Repository URL for solo.")
	ociShared := strings.Index(text, "OCI repository URL for shared.")
	if sharedAt < 0 || otherAt < sharedAt || solo < otherAt || ociShared < solo {
		t.Fatalf("rules are out of order")
	}
}

func TestRenderEscapesAQuoteInTheChartName(t *testing.T) {
	got, err := Render([]charts.Chart{
		{ID: "q", Repo: "https://example.test", RepoName: "q", Chart: "o'clock"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `depName = 'o\\'clock'`) {
		t.Fatalf("quote was not escaped:\n%s", got)
	}
}
