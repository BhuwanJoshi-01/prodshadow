package version

import (
	"strings"
	"testing"
)

func TestGetReportsGoVersionAndPlatform(t *testing.T) {
	info := Get()
	if info.GoVersion == "" {
		t.Fatal("expected Go version to be populated")
	}
	if !strings.Contains(info.Platform, "/") {
		t.Fatalf("platform %q should be GOOS/GOARCH", info.Platform)
	}
}

func TestStringTruncatesCommit(t *testing.T) {
	info := Info{Version: "v1.2.3", Commit: "0123456789abcdef", GoVersion: "go1.26", Platform: "linux/amd64"}
	got := info.String()
	if !strings.Contains(got, "0123456789ab") || strings.Contains(got, "0123456789abc") {
		t.Fatalf("unexpected commit rendering: %s", got)
	}
	if !strings.HasPrefix(got, "prodshadow v1.2.3") {
		t.Fatalf("unexpected prefix: %s", got)
	}
}

func TestStringUnknownCommit(t *testing.T) {
	got := Info{Version: "dev"}.String()
	if !strings.Contains(got, "commit unknown") {
		t.Fatalf("expected unknown commit marker, got %s", got)
	}
}
