package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"example.com/crossplane-preview-platform/evaluator/internal/preview"
)

func main() {
	gitops := flag.String("gitops", "", "trusted GitOps checkout containing status records")
	name := flag.String("name", "", "preview name, such as incident-tracker-pr-11")
	target := flag.String("target", "ready", "target phase: ready or deleted")
	mode := flag.String("expect-mode", "", "expected mode: namespace or vcluster")
	port := flag.Int("preview-port", 8088, "local ingress port")
	kubeContext := flag.String("kube-context", "kind-preview-platform", "Kubernetes context used for XR readiness checks")
	interval := flag.Duration("interval", 5*time.Second, "status polling interval")
	timeout := flag.Duration("timeout", 15*time.Minute, "maximum observation time")
	out := flag.String("out", "", "optional JSON output file")
	flag.Parse()
	if *gitops == "" || *name == "" || *port < 1 || *port > 65535 || *timeout <= 0 || *kubeContext == "" {
		fmt.Fprintln(os.Stderr, "require -gitops, -name, -kube-context, valid -preview-port and positive -timeout")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	store := preview.Store{Root: *gitops, PreviewPort: *port, Readiness: preview.KubectlReadinessChecker{Context: *kubeContext}}
	sample, err := store.ObservePhase(ctx, *name, *target, *mode, *interval, &http.Client{Timeout: 3 * time.Second})
	data, jsonErr := json.MarshalIndent(sample, "", "  ")
	if jsonErr != nil {
		fmt.Fprintln(os.Stderr, jsonErr)
		os.Exit(1)
	}
	data = append(data, '\n')
	if *out != "" {
		if mkErr := os.MkdirAll(filepath.Dir(*out), 0o755); mkErr != nil {
			fmt.Fprintln(os.Stderr, mkErr)
			os.Exit(1)
		}
		if writeErr := os.WriteFile(*out, data, 0o600); writeErr != nil {
			fmt.Fprintln(os.Stderr, writeErr)
			os.Exit(1)
		}
	}
	_, _ = os.Stdout.Write(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
