// Package release is the pinned Stageproof binary the index runs.
package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// Version is the Stageproof release every index report is built with.
	Version = "v0.5.0"
	// LinuxAMD64SHA256 is the checksum of stageproof_0.5.0_linux_amd64.tar.gz in that release.
	LinuxAMD64SHA256 = "4fa6b945b05386c9a3cfc5f9bea832668f0308df5f800ad1249a46f449f5279a"
	// LinuxAMD64BinarySHA256 is the checksum of the stageproof binary inside that archive.
	LinuxAMD64BinarySHA256 = "559e53045ab2d5f51844b1371f262fdf10394119c2169bcfb4748fa6aef93a64"
)

// AssetURL is the GitHub release archive for this OS and architecture.
func AssetURL() (url, checksum string, err error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", "", fmt.Errorf("pinned binary download is implemented for linux/amd64, not %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return "https://github.com/stageproof/stageproof/releases/download/" + Version + "/stageproof_" + strings.TrimPrefix(Version, "v") + "_linux_amd64.tar.gz", LinuxAMD64SHA256, nil
}

// Ensure downloads and verifies the pinned binary into destDir when it is not already there.
// An explicit path skips the download.
func Ensure(client *http.Client, explicit, destDir string) (string, error) {
	if explicit != "" {
		st, err := os.Stat(explicit)
		if err != nil {
			return "", err
		}
		if st.IsDir() {
			return "", fmt.Errorf("%s is a directory", explicit)
		}
		return explicit, nil
	}
	url, sum, err := AssetURL()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, "stageproof")
	if fileMatchesSHA256(dest, LinuxAMD64BinarySHA256) {
		return dest, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return "", err
	}
	got := sha256.Sum256(body)
	if hex.EncodeToString(got[:]) != sum {
		return "", fmt.Errorf("checksum mismatch for %s", url)
	}
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if filepath.Base(hdr.Name) != "stageproof" || hdr.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return "", err
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(f, io.LimitReader(tr, 256<<20)); err != nil {
			return "", errors.Join(err, f.Close())
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		if !fileMatchesSHA256(dest, LinuxAMD64BinarySHA256) {
			return "", errors.Join(fmt.Errorf("binary checksum mismatch for %s", url), os.Remove(dest))
		}
		return dest, nil
	}
	return "", fmt.Errorf("archive %s has no stageproof binary", url)
}

func fileMatchesSHA256(path, want string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == want
}
