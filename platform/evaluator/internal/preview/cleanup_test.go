package preview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupObserverRetainsFoundResourcesWhenLaterProbeFails(t *testing.T) {
	bin := t.TempDir()
	stub := `#!/bin/sh
case "$*" in
  *"get applications.argoproj.io"*)
    echo application.argoproj.io/preview-incident-tracker-pr-42
    ;;
  *"get previewenvironments.preview.platform.example.org"*)
    echo 'temporary API failure' >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	remaining, err := (KubectlCleanupObserver{Context: "test", PreviewPort: 8088}).Remaining(context.Background(), "incident-tracker-pr-42")
	if err == nil || !strings.Contains(err.Error(), "temporary API failure") {
		t.Fatalf("expected a visible observer error, got %v", err)
	}
	if len(remaining) != 1 || remaining[0] != "applications.argoproj.io/preview-incident-tracker-pr-42" {
		t.Fatalf("lost a resource discovered before the error: %v", remaining)
	}
}
