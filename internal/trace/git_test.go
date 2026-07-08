package trace

import (
	"strings"
	"testing"
)

func TestNormalizeEvidencePath(t *testing.T) {
	got := normalizeEvidencePath("/repo", "/repo/internal", "/repo/internal/fold/types.go")
	if strings.Join(got, ",") != "internal/fold/types.go" {
		t.Fatalf("absolute path: got %v", got)
	}

	got = normalizeEvidencePath("/repo", "/repo/internal", "./fold/types.go")
	if strings.Join(got, ",") != "fold/types.go,internal/fold/types.go" {
		t.Fatalf("relative path: got %v", got)
	}
}
