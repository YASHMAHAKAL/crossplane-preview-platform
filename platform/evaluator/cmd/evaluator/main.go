package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"example.com/crossplane-preview-platform/evaluator/internal/preview"
)

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func main() {
	snapshotPath := flag.String("snapshot", "", "PR snapshot JSON")
	configPath := flag.String("config", "", "trusted evaluator configuration JSON")
	gitopsRoot := flag.String("gitops", "", "trusted GitOps working tree")
	prNumber := flag.Int("pr", 0, "fetch this PR from GitHub instead of reading a snapshot file")
	tokenEnv := flag.String("github-token-env", "GITHUB_TOKEN", "environment variable containing a read-only GitHub token")
	publish := flag.Bool("publish", false, "commit and push evaluator output from a dedicated trusted GitOps checkout")
	flag.Parse()
	if (*snapshotPath == "") == (*prNumber == 0) || *configPath == "" || *gitopsRoot == "" {
		fmt.Fprintln(os.Stderr, "usage: evaluator (-snapshot pr.json | -pr 42) -config config.json -gitops /path/to/gitops")
		os.Exit(2)
	}
	var snapshot preview.Snapshot
	var config preview.Config
	if err := readJSON(*configPath, &config); err != nil {
		fail(err)
	}
	store := preview.Store{Root: *gitopsRoot}
	if *snapshotPath != "" {
		if err := readJSON(*snapshotPath, &snapshot); err != nil {
			fail(err)
		}
	} else {
		name, err := preview.PreviewName(config.Service, *prNumber)
		if err != nil {
			fail(err)
		}
		active, err := store.ActiveCount(name)
		if err != nil {
			fail(err)
		}
		client := &http.Client{Timeout: 30 * time.Second}
		reader := preview.GitHubReader{Client: client, Token: os.Getenv(*tokenEnv)}
		if reader.Token == "" {
			fail(fmt.Errorf("%s is required for GitHub mode", *tokenEnv))
		}
		snapshot, err = reader.Snapshot(context.Background(), config, *prNumber, active)
		if err != nil {
			fail(err)
		}
	}
	result, err := store.Reconcile(snapshot, config)
	if err != nil {
		fail(err)
	}
	if *publish {
		if err := store.Publish(context.Background(), fmt.Sprintf("Reconcile %s PR #%d", config.Service, snapshot.PR.Number)); err != nil {
			fail(err)
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
