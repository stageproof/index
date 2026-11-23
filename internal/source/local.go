package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/pairs"
)

// Local reads chart directories that already exist on disk.
// Versions maps a chart id to version names. Dirs maps "id@version" to a chart directory.
type Local struct {
	VersionsByID map[string][]pairs.Version
	Dirs         map[string]string
}

func (l Local) Versions(_ context.Context, chart charts.Chart) ([]pairs.Version, error) {
	got := append([]pairs.Version(nil), l.VersionsByID[chart.ID]...)
	if len(got) == 0 {
		return nil, fmt.Errorf("no local versions for %s", chart.ID)
	}
	return got, nil
}

func (l Local) Fetch(_ context.Context, chart charts.Chart, version string) (string, string, func() error, error) {
	dir, ok := l.Dirs[chart.ID+"@"+version]
	if !ok {
		return "", "", nil, fmt.Errorf("no local chart %s@%s", chart.ID, version)
	}
	sum, err := hashTree(dir)
	if err != nil {
		return "", "", nil, err
	}
	return dir, sum, func() error { return nil }, nil
}

func hashTree(root string) (string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, rel := range files {
		if _, err := io.WriteString(h, rel); err != nil {
			return "", err
		}
		buf, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		if _, err := h.Write(buf); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
