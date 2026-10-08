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
	kubeContext := flag.String("kube-context", "kind-preview-platform", "Kubernetes context used for read-only cleanup checks")
	previewPort := flag.Int("preview-port", 8088, "local ingress port for cleanup route checks")
	healthFile := flag.String("health-file", "", "optional local heartbeat JSON path outside GitOps")
	flag.Parse()
	if *configPath == "" || *gitopsRoot == "" || *interval < 10*time.Second || *kubeContext == "" || *previewPort < 1 || *previewPort > 65535 {
		log.Fatal("require -config, -gitops, valid -kube-context and -preview-port, and interval of at least 10s")
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
	observer := preview.KubectlCleanupObserver{Context: *kubeContext, PreviewPort: *previewPort}
	heartbeat := preview.WatcherHeartbeat{}
	if *healthFile != "" {
		previous, err := preview.ReadWatcherHeartbeat(*healthFile)
		if err == nil {
			heartbeat = previous
		} else if !os.IsNotExist(err) {
			log.Fatalf("read watcher heartbeat: %v", err)
		}
	}
	for {
		if *healthFile != "" {
			heartbeat.LastAttemptAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := preview.WriteWatcherHeartbeat(*healthFile, heartbeat); err != nil {
				log.Fatalf("write watcher heartbeat: %v", err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		err := reconcile(ctx, reader, store, config, *publish, observer)
		if *healthFile != "" {
			heartbeat.LastCompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err == nil {
				heartbeat.LastSuccessAt = heartbeat.LastCompletedAt
				heartbeat.LastError = ""
				heartbeat.ConsecutiveFailures = 0
			} else {
				heartbeat.LastError = err.Error()
				if len(heartbeat.LastError) > 512 {
					heartbeat.LastError = heartbeat.LastError[:512]
				}
				heartbeat.ConsecutiveFailures++
			}
			if writeErr := preview.WriteWatcherHeartbeat(*healthFile, heartbeat); writeErr != nil {
				log.Fatalf("write watcher heartbeat: %v", writeErr)
			}
		}
		if err != nil {
			log.Printf("reconcile: %v", err)
		}
		cancel()
		if *once {
			if err != nil {
				os.Exit(1)
			}
			return
		}
		time.Sleep(*interval)
	}
}

func reconcile(ctx context.Context, reader preview.GitHubReader, store preview.Store, config preview.Config, publish bool, observer preview.CleanupObserver) error {
	prs, err := reader.PullNumbers(ctx, config.Repository)
	if err != nil {
		return err
	}
	// Release capacity before evaluating newly opened PRs.
	sort.SliceStable(prs, func(i, j int) bool { return prs[i].State == "closed" && prs[j].State != "closed" })
	var failures []error
	var cleanupNames []string
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
		if snapshot.PR.State == "closed" || snapshot.PR.State == "merged" || (len(result.ReasonCodes) > 0 && result.ReasonCodes[0] == "ttl-expired") {
			cleanupNames = append(cleanupNames, name)
		}
	}
	if publish {
		if err := store.Publish(ctx, "Reconcile PR previews"); err != nil {
			failures = append(failures, fmt.Errorf("publish GitOps removal before cleanup check: %w", err))
			return fmt.Errorf("%d reconciliation error(s); first: %w", len(failures), failures[0])
		}
		for _, name := range cleanupNames {
			remaining, observationErr := observer.Remaining(ctx, name)
			result, err := store.ObserveCleanup(name, remaining, observationErr)
			if err != nil {
				failures = append(failures, fmt.Errorf("cleanup %s: %w", name, err))
				continue
			}
			log.Printf("cleanup %s phase=%s reason=%v evidence=%v", name, result.Phase, result.ReasonCodes, result.Evidence)
			if observationErr != nil {
				failures = append(failures, fmt.Errorf("cleanup %s: %w", name, observationErr))
			}
		}
		if err := store.Publish(ctx, "Record observed PR preview cleanup"); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d reconciliation error(s); first: %w", len(failures), failures[0])
	}
	return nil
}
