// Package source fetches chart versions and unpacked chart directories.
package source

import (
	"context"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/pairs"
)

// Source lists versions and unpacks one chart version.
type Source interface {
	Versions(ctx context.Context, chart charts.Chart) ([]pairs.Version, error)
	Fetch(ctx context.Context, chart charts.Chart, version string) (dir string, sum string, cleanup func() error, err error)
}
