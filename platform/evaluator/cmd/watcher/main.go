package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"example.com/crossplane-preview-platform/evaluator/internal/preview"
)

func main() {
	configPath := flag.String("config", "", "trusted evaluator configuration JSON")
	gitopsRoot := flag.String("gitops", "", "dedicated trusted GitOps checkout")
	interval := flag.Duration("interval", 30*time.Second, "GitHub polling interval")
	once := flag.Bool("once", false, "run one reconciliation pass")
	publish := flag.Bool("publish", true, "commit and push evaluator output after each pass")
	flag.Parse()
	if *configPath == "" || *gitopsRoot == "" || *interval < 10*time.Second {
		log.Fatal("require -config, -gitops, and interval of at least 10s")
	}
	data, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	var config preview.Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatal(err)
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is required")
	}
	reader := preview.GitHubReader{Client: &http.Client{Timeout: 30 * time.Second}, Token: token}
	store := preview.Store{Root: *gitopsRoot}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		if err := reconcile(ctx, reader, store, config, *publish); err != nil {
			log.Printf("reconcile: %v", err)
		}
		cancel()
		if *once {
			return
		}
		time.Sleep(*interval)
	}
}

func reconcile(ctx context.Context, reader preview.GitHubReader, store preview.Store, config preview.Config, publish bool) error {
	prs, err := reader.PullNumbers(ctx, config.Repository)
	if err != nil {
		return err
	}
	// Release capacity before evaluating newly opened PRs.
	sort.SliceStable(prs, func(i, j int) bool { return prs[i].State == "closed" && prs[j].State != "closed" })
	var failures []error
	for _, pr := range prs {
		name, err := preview.PreviewName(config.Service, pr.Number)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		active, err := store.ActiveCount(name)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		snapshot, err := reader.Snapshot(ctx, config, pr.Number, active)
		if err != nil {
			failures = append(failures, fmt.Errorf("PR #%d: %w", pr.Number, err))
			continue
		}
		result, err := store.Reconcile(snapshot, config)
		if err != nil {
			failures = append(failures, fmt.Errorf("PR #%d: %w", pr.Number, err))
			continue
		}
		log.Printf("PR #%d head=%s phase=%s mode=%s reason=%v", pr.Number, snapshot.PR.HeadSHA, result.Phase, result.Mode, result.ReasonCodes)
	}
	if publish {
		if err := store.Publish(ctx, "Reconcile PR previews"); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d reconciliation error(s); first: %w", len(failures), failures[0])
	}
	return nil
}
