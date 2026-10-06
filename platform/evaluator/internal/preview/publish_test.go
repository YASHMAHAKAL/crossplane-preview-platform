package preview

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishEmptyGitOpsCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	root := filepath.Join(base, "checkout")
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	run("init", "--bare", "-b", "main", remote)
	run("init", "-b", "main", root)
	run("-C", root, "config", "user.name", "Preview Evaluator")
	run("-C", root, "config", "user.email", "preview@example.invalid")
	run("-C", root, "remote", "add", "origin", remote)
	store := Store{Root: root}
	if err := store.Publish(context.Background(), "Initialize preview GitOps"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"previews/.gitkeep", "status/.gitkeep"} {
		command := exec.Command("git", "--git-dir", remote, "cat-file", "-e", "main:"+path)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("missing published %s: %v: %s", path, err, output)
		}
	}
	if err := store.Publish(context.Background(), "Retry preview GitOps"); err != nil {
		t.Fatalf("idempotent publish failed: %v", err)
	}
}

func TestValidatePublishTree(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	previewDir := filepath.Join(root, "previews", "incident-tracker-pr-42")
	statusDir := filepath.Join(root, "status")
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(previewDir, "previewenvironment.json"))
	write(filepath.Join(statusDir, "incident-tracker-pr-42.json"))
	if err := store.validatePublishTree(); err != nil {
		t.Fatalf("valid evaluator tree rejected: %v", err)
	}

	stray := filepath.Join(previewDir, "raw-pr-manifest.yaml")
	write(stray)
	if err := store.validatePublishTree(); err == nil || !strings.Contains(err.Error(), "unexpected path") {
		t.Fatalf("raw PR manifest accepted: %v", err)
	}
	if err := os.Remove(stray); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(statusDir, "incident-tracker-pr-43.json")
	if err := os.Symlink(filepath.Join(root, "outside"), link); err != nil {
		t.Fatal(err)
	}
	if err := store.validatePublishTree(); err == nil || !strings.Contains(err.Error(), "unexpected path") {
		t.Fatalf("symlink accepted: %v", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(statusDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(previewDir, statusDir); err != nil {
		t.Fatal(err)
	}
	if err := store.validatePublishTree(); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("symlink root accepted: %v", err)
	}
}
