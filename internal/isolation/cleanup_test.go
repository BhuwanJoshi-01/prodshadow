package isolation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRemoveOrphansRefusesUnlabelledContainers(t *testing.T) {
	p := newContainerProvider("docker", "")
	var removed []string
	p.run = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		switch args[0] {
		case "ps":
			return []byte("aaa\nbbb\n"), nil, nil
		case "rm":
			removed = append(removed, args[len(args)-1])
		}
		return nil, nil, nil
	}
	err := RemoveOrphans(context.Background(), p, []string{"aaa", "production-db"})
	if err == nil || !strings.Contains(err.Error(), "production-db is not a ProdShadow container") {
		t.Fatalf("expected refusal, got %v", err)
	}
	if len(removed) != 1 || removed[0] != "aaa" {
		t.Fatalf("removed %v", removed)
	}
}

func TestLocalProviderHasNoOrphans(t *testing.T) {
	if _, err := Orphans(context.Background(), newLocalProvider(""), ""); !errors.Is(err, ErrNoCleanup) {
		t.Fatalf("got %v", err)
	}
}
