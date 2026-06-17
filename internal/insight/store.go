package insight

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/config"
)

type ReportMeta struct {
	Name      string
	Path      string
	Scope     string
	Size      int64
	CreatedAt time.Time
}

func InsightsDir() string {
	return filepath.Join(config.DataDir(), "insights")
}

func SaveReport(name string, data []byte) (string, error) {
	dir := InsightsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create insights dir: %w", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return path, nil
}

func ListReports() ([]ReportMeta, error) {
	dir := InsightsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var reports []ReportMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		scope := parseScopeFromFilename(e.Name())
		reports = append(reports, ReportMeta{
			Name:      e.Name(),
			Path:      filepath.Join(dir, e.Name()),
			Scope:     scope,
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})
	return reports, nil
}

func parseScopeFromFilename(name string) string {
	name = strings.TrimSuffix(name, ".html")
	parts := strings.SplitN(name, "-", 4)
	if len(parts) >= 4 {
		return parts[3]
	}
	return name
}
