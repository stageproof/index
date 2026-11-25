package census

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCountsPairsAndIgnoresAFinding(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "v1", "charts", "fixtures", "web")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("meta.json", `{"versions":["0.1.0","0.2.0","0.3.0"]}`)
	write("0.1.0..0.2.0.json", `{
	  "stageproof_version": "v0.3.1",
	  "reproduce_command": "stageproof check --repo https://charts.example/fixtures --chart web --from 0.1.0 --to 0.2.0",
	  "k8s_results": {"1.35": {"verdict": "PASSED", "findings": [
	    {"rule_id": "impact.restart", "severity": "info", "message": "pods restart"}
	  ]}}
	}`)
	write("0.2.0..0.3.0.json", `{
	  "stageproof_version": "v0.3.0",
	  "reproduce_command": "stageproof check --repo https://charts.example/fixtures --chart web --from 0.2.0 --to 0.3.0",
	  "k8s_results": {"1.35": {"verdict": "BLOCKED", "findings": [
	    {"rule_id": "impact.delete.pvc", "severity": "blocker", "message": "Helm will delete PersistentVolumeClaim data."},
	    {"rule_id": "impact.delete.pvc", "severity": "blocker", "message": "Helm will delete PersistentVolumeClaim logs."},
	    {"rule_id": "k8s.api.removed", "severity": "blocker", "ignored": true, "message": "ignored"}
	  ]}}
	}`)
	other := filepath.Join(dir, "v1", "charts", "fixtures", "held")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "1.0.0..1.0.1.json"), []byte(`{
	  "stageproof_version": "v0.3.1",
	  "reproduce_command": "stageproof check --repo https://charts.example/fixtures --chart held --from 1.0.0 --to 1.0.1",
	  "k8s_results": {"1.35": {"verdict": "NOT CHECKED", "findings": []}}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	index := `{
	  "charts": [
	    {"id": "fixtures/web", "pairs": 2},
	    {"id": "fixtures/held", "pairs": 1},
	    {"id": "missing/chart", "pairs": 0, "error": "index.yaml: 404 Not Found"}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(dir, "v1", "index.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Catalog != 3 || len(sum.WithoutPairs) != 1 || sum.WithoutPairs[0].ID != "missing/chart" {
		t.Fatalf("catalog %+v", sum.WithoutPairs)
	}
	if sum.Charts != 2 || sum.Pairs != 3 || sum.Blocked != 1 || sum.NotChecked != 1 || sum.Passed != 1 {
		t.Fatalf("summary %+v", sum)
	}
	if len(sum.Rules) != 1 || sum.Rules[0].ID != "impact.delete.pvc" || sum.Rules[0].Pairs != 1 || sum.Rules[0].Findings != 2 {
		t.Fatalf("rules %+v", sum.Rules)
	}
	if sum.Rules[0].Example.Chart != "web" || !strings.Contains(sum.Rules[0].Example.Command, "--chart web") {
		t.Fatalf("example %+v", sum.Rules[0].Example)
	}
	if sum.Steps.Blocked != 1 || sum.Steps.Passed != 0 || sum.Steps.None != 1 || sum.Causes.Hazard != 1 {
		t.Fatalf("steps %+v causes %+v", sum.Steps, sum.Causes)
	}
	if len(sum.Minors) != 1 || sum.Minors[0] != "1.35" || sum.Accuracy.Commit == "" {
		t.Fatalf("minors %v accuracy %+v", sum.Minors, sum.Accuracy)
	}
	text := Markdown(sum)
	if strings.Contains(text, "safe") || strings.Contains(text, "\u2014") || !strings.Contains(text, "Helm will delete PersistentVolumeClaim data.") {
		t.Fatalf("markdown\n%s", text)
	}
}

func TestBlockedMinorWinsOverNotChecked(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "v1", "charts", "fixtures", "web")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{
	  "stageproof_version": "v0.3.1",
	  "reproduce_command": "stageproof check --chart web",
	  "k8s_results": {
	    "1.34": {"verdict": "NOT CHECKED"},
	    "1.35": {"verdict": "BLOCKED", "findings": [
	      {"rule_id": "supply.image.not_found", "severity": "blocker", "message": "Image missing."}
	    ]}
	  }
	}`
	if err := os.WriteFile(filepath.Join(root, "1.0.0..1.1.0.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Blocked != 1 || sum.NotChecked != 0 || sum.Rules[0].Example.Message != "Image missing." {
		t.Fatalf("summary %+v", sum)
	}
}

func TestNewestTargetAndBlockedCauses(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, "v1", "charts", rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("aaa/old/meta.json", `{"versions":["1.0.0","1.1.0"]}`)
	write("aaa/old/1.0.0..1.1.0.json", `{
	  "stageproof_version": "v0.4.0",
	  "reproduce_command": "stageproof check --chart old",
	  "k8s_results": {"1.35": {"verdict": "BLOCKED", "findings": [
	    {"rule_id": "k8s.api.removed", "severity": "blocker", "message": "old removed"}
	  ]}}
	}`)
	write("zzz/new/meta.json", `{"versions":["9.0.0","9.1.0","9.2.0"]}`)
	write("zzz/new/9.0.0..9.2.0.json", `{
	  "stageproof_version": "v0.4.0",
	  "reproduce_command": "stageproof check --chart new-jump",
	  "k8s_results": {"1.35": {"verdict": "BLOCKED", "findings": [
	    {"rule_id": "k8s.api.removed", "severity": "blocker", "message": "jump removed"}
	  ]}}
	}`)
	write("zzz/new/9.1.0..9.2.0.json", `{
	  "stageproof_version": "v0.4.0",
	  "reproduce_command": "stageproof check --chart new",
	  "k8s_results": {"1.35": {"verdict": "BLOCKED", "findings": [
	    {"rule_id": "k8s.api.removed", "severity": "blocker", "message": "new removed"},
	    {"rule_id": "impact.delete.pvc", "severity": "blocker", "message": "new delete"}
	  ]}}
	}`)
	write("mmm/render/meta.json", `{"versions":["0.1.0","0.2.0"]}`)
	write("jjj/jump/meta.json", `{"versions":["1.0.0","2.0.0","3.0.0"]}`)
	write("jjj/jump/1.0.0..3.0.0.json", `{
	  "stageproof_version": "v0.4.0",
	  "reproduce_command": "stageproof check --chart jump-only",
	  "k8s_results": {"1.35": {"verdict": "PASSED", "findings": []}}
	}`)
	write("mmm/render/0.1.0..0.2.0.json", `{
	  "stageproof_version": "v0.4.0",
	  "reproduce_command": "stageproof check --chart render",
	  "k8s_results": {"1.35": {"verdict": "PASSED", "findings": []}, "1.36": {"verdict": "BLOCKED", "findings": [
	    {"rule_id": "helm.values.new_required_unset", "severity": "blocker", "message": "port is required"}
	  ]}}
	}`)
	sum, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Steps.Blocked != 3 || sum.Steps.None != 1 || sum.Steps.Passed != 0 || sum.Charts != 4 {
		t.Fatalf("steps %+v charts %d", sum.Steps, sum.Charts)
	}
	if sum.Causes.Removed != 2 || sum.Causes.Render != 1 || sum.Causes.Mixed != 1 || sum.Causes.Hazard != 0 {
		t.Fatalf("causes %+v", sum.Causes)
	}
	var removed Example
	for _, rule := range sum.Rules {
		if rule.ID == "k8s.api.removed" {
			removed = rule.Example
		}
	}
	if removed.Chart != "new" || removed.To != "9.2.0" || removed.From != "9.1.0" {
		t.Fatalf("example %+v", removed)
	}
	text := Markdown(sum)
	for _, want := range []string{
		"Kubernetes minors 1.35, 1.36",
		"only a removed API | 2",
		"only a chart-defaults render problem | 1",
		"more than one of those | 1",
		sum.Accuracy.Commit,
		"9.1.0 to 9.2.0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s\n%s", want, text)
		}
	}
	for _, banned := range []string{"0 of 281", "45065690496", "Flux replay: 70 of 70 across 5 public repositories", "\u2014"} {
		if strings.Contains(text, banned) {
			t.Fatalf("report still contains %s", banned)
		}
	}
	src, err := os.ReadFile("census.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"0 of 281", "0 of 283", "042579b", "18 of 18"} {
		if strings.Contains(string(src), banned) {
			t.Fatalf("census.go contains measurement text %s", banned)
		}
	}
}
