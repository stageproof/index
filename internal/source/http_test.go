package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stageproof/index/internal/charts"
)

func TestHelmFetchVerifiesDigestAndCleansUp(t *testing.T) {
	chart := packChart(t, "web", "1.2.3")
	sum := sha256.Sum256(chart)
	digest := hex.EncodeToString(sum[:])
	downloads := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("apiVersion: v1\nentries:\n  web:\n    - version: 1.2.3\n      created: 2026-01-02T03:04:05Z\n      digest: sha256:" + digest + "\n      urls:\n        - charts/web-1.2.3.tgz\n"))
	})
	mux.HandleFunc("/charts/web-1.2.3.tgz", func(w http.ResponseWriter, r *http.Request) {
		downloads++
		_, _ = w.Write(chart)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	h := &HTTP{Client: srv.Client(), CacheDir: t.TempDir()}
	item := charts.Chart{ID: "example/web", Repo: srv.URL, RepoName: "example", Chart: "web"}
	versions, err := h.Versions(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Name != "1.2.3" || versions[0].Created.IsZero() {
		t.Fatalf("versions %#v", versions)
	}
	dir, got, cleanup, err := h.Fetch(context.Background(), item, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Fatalf("digest %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "Chart.yaml")); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(dir)
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("temp dir %s still exists", parent)
	}
	if _, _, cleanup2, err := h.Fetch(context.Background(), item, "1.2.3"); err != nil {
		t.Fatal(err)
	} else if err := cleanup2(); err != nil {
		t.Fatal(err)
	}
	if downloads != 1 {
		t.Fatalf("chart archive was downloaded %d times", downloads)
	}
}

func TestOCITagsUseTheBearerChallenge(t *testing.T) {
	chart := packChart(t, "web", "1.2.3")
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"test-token"}`))
	})
	mux.HandleFunc("/v2/example/web/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.Header().Set("Www-Authenticate", `Bearer realm="http://`+r.Host+`/token",service="registry",scope="repository:example/web:pull"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"tags":["1.2.3","1.3.0-rc.1"]}`))
	})
	mux.HandleFunc("/v2/example/web/manifests/1.2.3", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"layers":[{"mediaType":"application/vnd.cncf.helm.chart.content.v1.tar+gzip","digest":"sha256:abc"}]}`))
	})
	mux.HandleFunc("/v2/example/web/blobs/sha256:abc", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(chart)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	h := &HTTP{Client: srv.Client(), Scheme: "http"}
	item := charts.Chart{ID: "example/web", Repo: "oci://" + srv.Listener.Addr().String() + "/example", RepoName: "example", Chart: "web"}
	versions, err := h.Versions(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("tags %#v", versions)
	}
	dir, _, cleanup, err := h.Fetch(context.Background(), item, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := os.Stat(filepath.Join(dir, "Chart.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestUntarRejectsParentPaths(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := []byte("nope")
	hdr := &tar.Header{Name: "../evil", Mode: 0o644, Size: int64(len(body))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err := untarChart(buf.Bytes(), dest); err == nil {
		t.Fatal("expected the archive to be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil")); !os.IsNotExist(err) {
		t.Fatal("escaped file was written")
	}
}

func packChart(t *testing.T, name, version string) []byte {
	t.Helper()
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	write := func(path, body string) {
		t.Helper()
		hdr := &tar.Header{Name: path, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write(name+"/Chart.yaml", "apiVersion: v2\nname: "+name+"\nversion: "+version+"\n")
	write(name+"/templates/cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: web\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}
