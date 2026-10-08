package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"example.com/crossplane-preview-platform/evaluator/internal/preview"
)

func main() {
	root := flag.String("gitops", "", "trusted GitOps working tree with evaluator status records")
	listen := flag.String("listen", "127.0.0.1:8090", "status API listen address")
	previewPort := flag.Int("preview-port", 8088, "host port mapped to the local preview ingress")
	kubeContext := flag.String("kube-context", "kind-preview-platform", "Kubernetes context used for XR readiness checks")
	healthFile := flag.String("watcher-health-file", "", "local watcher heartbeat JSON path")
	flag.Parse()
	if *root == "" || *previewPort < 1 || *previewPort > 65535 || *kubeContext == "" {
		log.Fatal("-gitops, -kube-context and a valid -preview-port are required")
	}
	routes := http.NewServeMux()
	routes.Handle("/api/previews/", (preview.Store{Root: *root, PreviewPort: *previewPort, Readiness: preview.KubectlReadinessChecker{Context: *kubeContext}}).StatusHandler(&http.Client{Timeout: 2 * time.Second}))
	routes.Handle("/healthz", preview.WatcherHealthHandler(*healthFile, 3*time.Minute))
	server := &http.Server{Addr: *listen, Handler: routes, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
