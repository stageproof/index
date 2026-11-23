package charts

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRejectsUnsafeSchemeAndWhitespace(t *testing.T) {
	bad := []Chart{
		{ID: "a/b", Repo: "file:///etc/passwd", RepoName: "a", Chart: "b"},
		{ID: "a/b", Repo: "https://charts.example/repo --set foo=bar", RepoName: "a", Chart: "b"},
		{ID: "a/b", Repo: "https://charts.example", RepoName: "a", Chart: "b c"},
	}
	for _, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("expected validation error for %#v", c)
		}
	}
}

func TestWriteRoundTripKeepsTheReviewNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charts.yaml")
	in := File{Charts: []Chart{{
		ID: "acme/web", Repo: "https://charts.example", RepoName: "acme", Chart: "web", Stars: 3,
	}}}
	if err := Write(path, in); err != nil {
		t.Fatal(err)
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf, []byte("# Candidate list from Artifact Hub. Review it before the nightly build.\n")) {
		t.Fatalf("header missing:\n%s", buf)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Charts) != 1 || got.Charts[0].ID != "acme/web" {
		t.Fatalf("%#v", got)
	}
}
