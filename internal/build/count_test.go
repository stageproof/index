package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountPairFilesKeepsOlderReports(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("meta.json", `{"versions":["0.1.0","0.2.0","0.3.0"]}`)
	write("0.1.0..0.2.0.json", `{"k8s_results":{"1.35":{"verdict":"PASSED","exit_code":0,"findings":[]}}}`)
	write("0.2.0..0.3.0.json", `{"k8s_results":{"1.35":{"verdict":"BLOCKED","exit_code":1,"findings":[]}}}`)
	pairs, blocked, err := countPairFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pairs != 2 || blocked != 1 {
		t.Fatalf("pairs %d blocked %d", pairs, blocked)
	}
}
