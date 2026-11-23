package charts

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

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
