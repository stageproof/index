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
	"github.com/stageproof/index/internal/release"
	"github.com/stageproof/index/internal/source"
)

func TestChoosePairsAddsJumpsOnlyWhenTheLatestChanges(t *testing.T) {
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	versions := []pairs.Version{{Name: "1.0.0", Created: now}, {Name: "1.1.0", Created: now}, {Name: "1.2.0", Created: now}}
	same := choosePairs(versions, now, 1, 20, "1.2.0")
	if len(same) != 1 || same[0].From != "1.1.0" || same[0].To != "1.2.0" {
		t.Fatalf("unchanged latest %#v", same)
	}
	changed := choosePairs(versions, now, 1, 20, "1.1.0")
	got := map[string]string{}
	for _, pair := range changed {
		got[pair.From] = pair.To
	}
	if len(changed) != 2 || got["1.0.0"] != "1.2.0" || got["1.1.0"] != "1.2.0" {
		t.Fatalf("released chart %#v", changed)
	}
}

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

func TestSelectShardUsesSortedIDs(t *testing.T) {
	list := []charts.Chart{
		{ID: "b/b", RepoName: "b", Chart: "b"},
		{ID: "a/a", RepoName: "a", Chart: "a"},
		{ID: "d/d", RepoName: "d", Chart: "d"},
		{ID: "c/c", RepoName: "c", Chart: "c"},
	}
	got, err := ShardSlugs(list, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a/a,c/c" {
		t.Fatalf("shard 0: %v", got)
	}
	got, err = ShardSlugs(list, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "b/b,d/d" {
		t.Fatalf("shard 1: %v", got)
	}
	shard, shards, err := ParseShard("3/40")
	if err != nil || shard != 3 || shards != 40 {
		t.Fatalf("parse %d/%d %v", shard, shards, err)
	}
	if _, _, err := ParseShard(""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseShard("2/2"); err == nil {
		t.Fatal("2/2 was accepted")
	}
	if _, _, err := ParseShard("nope"); err == nil {
		t.Fatal("bare shard was accepted")
	}
}

func TestCatalogCountsStoredPairs(t *testing.T) {
	root := t.TempDir()
	chart := charts.Chart{
		ID: "fixtures/web", Repo: "https://charts.example/fixtures", RepoName: "fixtures", Chart: "web", Stars: 4,
	}
	dir := filepath.Join(root, "v1", "charts", "fixtures", "web")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(`{"latest":"0.2.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0.1.0..0.2.0.json"), []byte(`{"k8s_results":{"1.35":{"verdict":"BLOCKED","exit_code":1}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0.2.0..0.3.0.json"), []byte(`{"k8s_results":{"1.35":{"verdict":"PASSED","exit_code":0}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteCatalog(charts.File{Charts: []charts.Chart{chart}}, root); err != nil {
		t.Fatal(err)
	}
	buf, err := os.ReadFile(filepath.Join(root, "v1", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version string `json:"stageproof_version"`
		Charts  []struct {
			Latest  string `json:"latest"`
			Pairs   int    `json:"pairs"`
			Blocked int    `json:"blocked"`
		} `json:"charts"`
	}
	if err := json.Unmarshal(buf, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != release.Version || len(doc.Charts) != 1 {
		t.Fatalf("catalog %#v", doc)
	}
	row := doc.Charts[0]
	if row.Latest != "0.2.0" || row.Pairs != 2 || row.Blocked != 1 {
		t.Fatalf("row %#v", row)
	}
}

func TestRecheckRebuildsStoredPairsOutsideTheWindow(t *testing.T) {
	bin := os.Getenv("STAGEPROOF_BIN")
	if bin == "" {
		t.Fatal("STAGEPROOF_BIN is required")
	}
	root := t.TempDir()
	chart := charts.Chart{ID: "fixtures/web", Repo: "https://charts.example/fixtures", RepoName: "fixtures", Chart: "web"}
	oldFrom := writeChart(t, root, "web", "1.0.0", "n: {{ .Chart.Version | quote }}\n")
	oldTo := writeChart(t, root, "web", "1.1.0", "n: {{ .Chart.Version | quote }}\n")
	keptFrom := writeChart(t, root, "web", "2.0.0", "n: {{ .Chart.Version | quote }}\n")
	keptTo := writeChart(t, root, "web", "2.1.0", "n: {{ .Chart.Version | quote }}\n")
	blankFrom := writeChart(t, root, "web", "3.0.0", "n: {{ .Chart.Version | quote }}\n")
	blankTo := writeChart(t, root, "web", "3.1.0", "n: {{ .Chart.Version | quote }}\n")
	latest := writeChart(t, root, "web", "9.0.0", "n: {{ .Chart.Version | quote }}\n")
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	src := source.Local{
		VersionsByID: map[string][]pairs.Version{chart.ID: {
			{Name: "1.0.0", Created: old},
			{Name: "1.1.0", Created: old},
			{Name: "2.0.0", Created: old},
			{Name: "2.1.0", Created: old},
			{Name: "3.0.0", Created: old},
			{Name: "3.1.0", Created: old},
			{Name: "9.0.0", Created: recent},
		}},
		Dirs: map[string]string{
			chart.ID + "@1.0.0": oldFrom, chart.ID + "@1.1.0": oldTo,
			chart.ID + "@2.0.0": keptFrom, chart.ID + "@2.1.0": keptTo,
			chart.ID + "@3.0.0": blankFrom, chart.ID + "@3.1.0": blankTo,
			chart.ID + "@9.0.0": latest,
		},
	}
	out := filepath.Join(root, "out")
	dir := filepath.Join(out, "v1", "charts", "fixtures", "web")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "1.0.0..1.1.0.json")
	staleBody := []byte(`{"stageproof_version":"v0.3.0","k8s_results":{"1.35":{"verdict":"BLOCKED"}}}`)
	if err := os.WriteFile(stale, staleBody, 0o644); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(dir, "2.0.0..2.1.0.json")
	keptBody := `{"stageproof_version":"` + release.Version + `","marker":"keep-this-body","k8s_results":{"1.35":{"verdict":"PASSED"}}}`
	if err := os.WriteFile(kept, []byte(keptBody), 0o644); err != nil {
		t.Fatal(err)
	}
	blank := filepath.Join(dir, "3.0.0..3.1.0.json")
	if err := os.WriteFile(blank, []byte(`{"k8s_results":{"1.35":{"verdict":"PASSED"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	opt := Options{
		OutDir: out, Binary: bin, Minors: []string{"1.35"}, APIServer: "off", Now: now, Source: src,
	}
	if _, err := Run(context.Background(), charts.File{Charts: []charts.Chart{chart}}, opt); err != nil {
		t.Fatal(err)
	}
	buf, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(staleBody) {
		t.Fatalf("stored pair was rebuilt without -recheck:\n%s", buf)
	}
	opt.Recheck = true
	if _, err := Run(context.Background(), charts.File{Charts: []charts.Chart{chart}}, opt); err != nil {
		t.Fatal(err)
	}
	buf, err = os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buf), release.Version) {
		t.Fatalf("stale pair was not rebuilt:\n%s", buf)
	}
	keptBuf, err := os.ReadFile(kept)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(keptBuf), "keep-this-body") {
		t.Fatalf("current pair was rebuilt:\n%s", keptBuf)
	}
	blankBuf, err := os.ReadFile(blank)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blankBuf), release.Version) {
		t.Fatalf("pair with no stageproof_version was not rebuilt:\n%s", blankBuf)
	}
}
