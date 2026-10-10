package preview

import (
	"context"
	"encoding/json"
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

func TestVClusterInstallerGrantIsFixedAndRemoved(t *testing.T) {
	snapshot, config := fixture()
	crd, err := os.ReadFile("../../../backstage/templates/request-preview/cluster/deploy/cluster/incident-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Files = append(snapshot.Files, ChangedFile{Path: "deploy/cluster/incident-policy.json", Content: string(crd)})
	store := Store{Root: t.TempDir()}
	decision, err := store.Reconcile(snapshot, config)
	if err != nil || decision.Phase != "approved" || decision.Mode != "vcluster" {
		t.Fatalf("vCluster decision: %+v %v", decision, err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	dir := filepath.Join(store.Root, "previews", name)
	if err := store.validatePublishTree(); err != nil {
		t.Fatalf("fixed grant rejected: %v", err)
	}
	roleBytes, err := os.ReadFile(filepath.Join(dir, installerRoleFile))
	if err != nil {
		t.Fatal(err)
	}
	var role map[string]any
	if err := json.Unmarshal(roleBytes, &role); err != nil {
		t.Fatal(err)
	}
	role["rules"] = []any{}
	if err := atomicJSON(filepath.Join(dir, installerRoleFile), role); err != nil {
		t.Fatal(err)
	}
	if err := store.validatePublishTree(); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("modified grant accepted: %v", err)
	}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	if err := store.validatePublishTree(); err != nil {
		t.Fatalf("repaired grant rejected: %v", err)
	}
	snapshot.PR.State = "closed"
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("grant directory remains after close: %v", err)
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
	snapshot, config := fixture()
	decision := Evaluate(snapshot, config)
	xr, err := MakeXR(snapshot, decision, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(filepath.Join(previewDir, "previewenvironment.json"), xr); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(statusDir, "incident-tracker-pr-42.json"))
	write(filepath.Join(statusDir, "incident-tracker-pr-42-c.json"))
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
