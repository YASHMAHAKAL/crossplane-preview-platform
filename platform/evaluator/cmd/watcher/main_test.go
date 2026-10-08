package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"example.com/crossplane-preview-platform/evaluator/internal/preview"
)

type noResources struct{}

func (noResources) Remaining(context.Context, string) ([]string, error) { return nil, nil }

func TestOnePRReadFailureDoesNotBlockOtherPublishedCleanup(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/example/source/pulls":
			_, _ = w.Write([]byte(`[{"number":1,"state":"closed"},{"number":2,"state":"closed"}]`))
		case "/repos/example/source/pulls/1":
			http.Error(w, "unavailable", http.StatusInternalServerError)
		case "/repos/example/source/pulls/2":
			_, _ = w.Write([]byte(`{"number":2,"state":"closed","user":{"login":"operator"},"head":{"sha":"abc123","repo":{"full_name":"example/source"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	root := filepath.Join(base, "checkout")
	git := func(args ...string) []byte {
		t.Helper()
		output, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return output
	}
	git("init", "--bare", "-b", "main", remote)
	git("init", "-b", "main", root)
	git("-C", root, "config", "user.name", "Preview Evaluator")
	git("-C", root, "config", "user.email", "preview@example.invalid")
	git("-C", root, "remote", "add", "origin", remote)
	store := preview.Store{Root: root}
	if err := store.Publish(context.Background(), "Initialize trusted GitOps"); err != nil {
		t.Fatal(err)
	}
	reader := preview.GitHubReader{Client: server.Client(), APIBase: server.URL}
	config := preview.Config{Service: "incident-tracker", Repository: "example/source"}
	err := reconcile(context.Background(), reader, store, config, true, noResources{})
	if err == nil || !strings.Contains(err.Error(), "PR #1") {
		t.Fatalf("expected failed PR read to remain visible, got %v", err)
	}
	status := string(git("--git-dir", remote, "show", "main:status/incident-tracker-pr-2.json"))
	if !strings.Contains(status, `"phase": "cleaning"`) || !strings.Contains(status, `"pr-closed"`) {
		t.Fatalf("healthy PR cleanup was not published: %s", status)
	}
}
