package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"example.com/crossplane-preview-platform/evaluator/internal/preview"
)

func command(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func output(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func run() error {
	configPath := flag.String("config", "config.example.json", "trusted evaluator configuration JSON")
	projectRoot := flag.String("project-root", "../..", "trusted checkout containing the isolation gate")
	prNumber := flag.Int("pr", 0, "open same-repository PR number")
	flag.Parse()
	if *prNumber < 1 {
		return errors.New("-pr must be a positive PR number")
	}
	root, err := filepath.Abs(*projectRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "deploy/local/verify-crossplane-isolation.sh")); err != nil {
		return fmt.Errorf("trusted isolation gate is missing: %w", err)
	}
	data, err := os.ReadFile(*configPath)
	if err != nil {
		return err
	}
	var config preview.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token, err = output(root, "gh", "auth", "token")
		if err != nil {
			return fmt.Errorf("get GitHub token: %w", err)
		}
	}
	reader := preview.GitHubReader{Client: &http.Client{Timeout: 30 * time.Second}, Token: token}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	record, err := reader.CandidateForPR(ctx, config, *prNumber)
	if err != nil {
		return err
	}
	resolved, err := output(root, "docker", "buildx", "imagetools", "inspect", record.PackageTag, "--format", "{{.Manifest.Digest}}")
	if err != nil {
		return fmt.Errorf("resolve candidate tag: %w", err)
	}
	if record.PackageDigest != strings.SplitN(record.PackageTag, ":", 2)[0]+"@"+resolved {
		return errors.New("candidate tag no longer resolves to the recorded digest")
	}
	temp, err := os.MkdirTemp("", "preview-candidate-pr-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	source := filepath.Join(temp, "source")
	url := "https://github.com/" + config.Repository + ".git"
	if err := command(root, os.Environ(), "git", "clone", "--quiet", "--no-tags", "--depth", "1", "--branch", record.Branch, url, source); err != nil {
		return fmt.Errorf("clone current PR branch: %w", err)
	}
	actual, err := output(source, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if actual != record.HeadSHA {
		return errors.New("PR branch moved before candidate source checkout")
	}
	fmt.Printf("candidate-gate: PR #%d head %s, package %s\n", *prNumber, record.HeadSHA, record.PackageDigest)
	gateEnv := append(os.Environ(), "PREVIEW_CANDIDATE_SOURCE_DIR="+source)
	if err := command(root, gateEnv, filepath.Join(root, "deploy/local/verify-crossplane-isolation.sh"), record.PackageDigest); err != nil {
		return fmt.Errorf("isolated candidate gate: %w", err)
	}
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer finalCancel()
	current, err := reader.CandidateForPR(finalCtx, config, *prNumber)
	if err != nil {
		return err
	}
	if current.HeadSHA != record.HeadSHA || current.PackageDigest != record.PackageDigest || current.WorkflowRunID != record.WorkflowRunID {
		return errors.New("PR head or candidate build changed during verification; rerun for the current head")
	}
	fmt.Printf("candidate-gate: PASS PR #%d at %s (workflow run %d)\n", *prNumber, record.HeadSHA, record.WorkflowRunID)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "candidate-gate:", err)
		os.Exit(1)
	}
}
