package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/pairs"
	"github.com/stageproof/index/internal/source"
)

func TestReproduceCommandNormalizesOCIRepo(t *testing.T) {
	cases := []struct {
		repo  string
		chart string
		want  string
	}{
		{"oci://registry-1.docker.io/cloudpirates/rabbitmq", "rabbitmq", "oci://registry-1.docker.io/cloudpirates"},
		{"oci://public.ecr.aws/karpenter/karpenter", "karpenter", "oci://public.ecr.aws/karpenter"},
		{"oci://registry-1.docker.io/cloudpirates", "rabbitmq", "oci://registry-1.docker.io/cloudpirates"},
		{"oci://ghcr.io/rabbitmq", "rabbitmq", "oci://ghcr.io/rabbitmq"},
		{"https://charts.example/rabbitmq", "rabbitmq", "https://charts.example/rabbitmq"},
	}
	for _, tc := range cases {
		if got := reproduceRepo(tc.repo, tc.chart); got != tc.want {
			t.Errorf("reproduceRepo(%q, %q) = %q, want %q", tc.repo, tc.chart, got, tc.want)
		}
	}
}

func TestDeterministicHundredCharts(t *testing.T) {
	bin := os.Getenv("STAGEPROOF_BIN")
	if bin == "" {
		t.Fatal("STAGEPROOF_BIN is required")
	}
	if _, err := exec.LookPath("helm"); err != nil {
		t.Fatal("helm is required on PATH")
	}
	root := t.TempDir()
	file, src := fixtureCharts(t, root, 100)
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	opt := Options{Binary: bin, Minors: []string{"1.35"}, APIServer: "off", Now: now, Source: src}
	ctx := context.Background()
	if _, err := Run(ctx, file, withOut(opt, left)); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, file, withOut(opt, right)); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("diff", "-r", left, right)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("builds differ: %v\n%s", err, out)
	}
	walkNoArchives(t, left)
}

func TestBlockedPairIsLabeledDefaultsOnly(t *testing.T) {
	bin := os.Getenv("STAGEPROOF_BIN")
	if bin == "" {
		t.Fatal("STAGEPROOF_BIN is required")
	}
	root := t.TempDir()
	from := writeChart(t, root, "web", "0.1.0", "n: {{ .Chart.Version | quote }}\n")
	to := writeChart(t, root, "web", "0.2.0", "ok: {{ required \"port\" .Values.port | quote }}\n")
	chart := charts.Chart{ID: "fixtures/web", Repo: "https://charts.example/fixtures", RepoName: "fixtures", Chart: "web"}
	file := charts.File{Charts: []charts.Chart{chart}}
	src := source.Local{
		VersionsByID: map[string][]pairs.Version{"fixtures/web": {{Name: "0.1.0"}, {Name: "0.2.0"}}},
		Dirs:         map[string]string{"fixtures/web@0.1.0": from, "fixtures/web@0.2.0": to},
	}
	out := filepath.Join(root, "out")
	res, err := Run(context.Background(), file, Options{
		OutDir: out, Binary: bin, Minors: []string{"1.35"}, APIServer: "off",
		Now: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), Source: src,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked != 1 || res.Pairs != 1 {
		t.Fatalf("result %#v", res)
	}
	buf, err := os.ReadFile(filepath.Join(out, "v1", "charts", "fixtures", "web", "0.1.0..0.2.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["notice"] != notice || doc["scope"] != "chart-defaults-only" {
		t.Fatalf("label %#v", doc)
	}
	if _, ok := doc["timestamp"]; ok {
		t.Fatal("timestamp was kept")
	}
	cmd, _ := doc["reproduce_command"].(string)
	if cmd != "stageproof check --repo https://charts.example/fixtures --chart web --from 0.1.0 --to 0.2.0 --values-proof full --current-k8s 1.35 --api-server off --format json" {
		t.Fatalf("command %s", cmd)
	}
	raw := string(buf)
	if strings.Contains(raw, from) || strings.Contains(raw, to) {
		t.Fatal("report kept a local chart path")
	}
}

type errSource struct {
	source.Local
	badID string
}

func (e errSource) Versions(ctx context.Context, chart charts.Chart) ([]pairs.Version, error) {
	if chart.ID == e.badID {
		return nil, fmt.Errorf("chart %s is not in %s", chart.Chart, chart.Repo)
	}
	return e.Local.Versions(ctx, chart)
}

func TestVersionListErrorKeepsReportsOnDisk(t *testing.T) {
	root := t.TempDir()
	chart := charts.Chart{ID: "fixtures/held", Repo: "https://charts.example/held", RepoName: "fixtures", Chart: "held"}
	dir := filepath.Join(root, "v1", "charts", chart.RepoName, chart.Chart)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"k8s_results":{"1.35":{"verdict":"BLOCKED","exit_code":1}}}`)
	if err := os.WriteFile(filepath.Join(dir, "1.0.0..1.0.1.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	row, err := buildChart(context.Background(), chart, Options{
		OutDir: root,
		Source: errSource{badID: chart.ID},
		Now:    now,
	}, []string{"1.35"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if row.Failed != 1 || row.Pairs != 1 || row.Blocked != 1 || row.Error == "" {
		t.Fatalf("row %#v", row)
	}
	if _, err := os.Stat(filepath.Join(dir, "1.0.0..1.0.1.json")); err != nil {
		t.Fatal(err)
	}
}

func TestChartErrorDoesNotStopTheCatalog(t *testing.T) {
	bin := os.Getenv("STAGEPROOF_BIN")
	if bin == "" {
		t.Fatal("STAGEPROOF_BIN is required")
	}
	root := t.TempDir()
	from := writeChart(t, root, "web", "0.1.0", "n: {{ .Chart.Version | quote }}\n")
	to := writeChart(t, root, "web", "0.2.0", "n: {{ .Chart.Version | quote }}\n")
	live := charts.Chart{ID: "fixtures/web", Repo: "https://charts.example/fixtures", RepoName: "fixtures", Chart: "web"}
	dead := charts.Chart{ID: "dead/repo", Repo: "https://charts.example/missing", RepoName: "dead", Chart: "repo"}
	file := charts.File{Charts: []charts.Chart{dead, live}}
	src := errSource{
		badID: dead.ID,
		Local: source.Local{
			VersionsByID: map[string][]pairs.Version{live.ID: {{Name: "0.1.0"}, {Name: "0.2.0"}}},
			Dirs:         map[string]string{live.ID + "@0.1.0": from, live.ID + "@0.2.0": to},
		},
	}
	out := filepath.Join(root, "out")
	res, err := Run(context.Background(), file, Options{
		OutDir: out, Binary: bin, Minors: []string{"1.35"}, APIServer: "off",
		Now: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), Source: src,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Charts != 2 || res.Failed != 1 || res.Pairs != 1 {
		t.Fatalf("result %#v", res)
	}
	if _, err := os.Stat(filepath.Join(out, "v1", "charts", "fixtures", "web", "0.1.0..0.2.0.json")); err != nil {
		t.Fatal(err)
	}
	buf, err := os.ReadFile(filepath.Join(out, "v1", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buf), "chart repo is not in https://charts.example/missing") {
		t.Fatalf("catalog lost the chart error:\n%s", buf)
	}
}

func withOut(opt Options, dir string) Options {
	opt.OutDir = dir
	return opt
}

func fixtureCharts(t *testing.T, root string, n int) (charts.File, source.Local) {
	t.Helper()
	file := charts.File{}
	src := source.Local{VersionsByID: map[string][]pairs.Version{}, Dirs: map[string]string{}}
	for i := 0; i < n; i++ {
		name := "chart-" + pad(i)
		id := "fixtures/" + name
		from := writeChart(t, root, name, "0.1.0", "n: {{ .Chart.Version | quote }}\n")
		to := writeChart(t, root, name, "0.2.0", "n: {{ .Chart.Version | quote }}\n")
		file.Charts = append(file.Charts, charts.Chart{
			ID: id, Repo: "https://charts.example/fixtures", RepoName: "fixtures", Chart: name, Stars: n - i,
		})
		src.VersionsByID[id] = []pairs.Version{{Name: "0.1.0"}, {Name: "0.2.0"}}
		src.Dirs[id+"@0.1.0"] = from
		src.Dirs[id+"@0.2.0"] = to
	}
	return file, src
}

func writeChart(t *testing.T, root, name, version, data string) string {
	t.Helper()
	dir := filepath.Join(root, "src", name+"-"+version)
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	chart := "apiVersion: v2\nname: " + name + "\ndescription: fixture\ntype: application\nversion: " + version + "\nappVersion: \"1\"\n"
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(chart), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: " + name + "\ndata:\n  " + data
	if err := os.WriteFile(filepath.Join(dir, "templates", "cm.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func walkNoArchives(t *testing.T, root string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".tgz" || filepath.Ext(path) == ".tar" {
			t.Fatalf("archive committed at %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func pad(n int) string {
	return strconv.Itoa(n/100) + strconv.Itoa((n/10)%10) + strconv.Itoa(n%10)
}
