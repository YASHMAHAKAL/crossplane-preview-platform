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
	flag.Parse()
	if *root == "" || *previewPort < 1 || *previewPort > 65535 {
		log.Fatal("-gitops and a valid -preview-port are required")
	}
	server := &http.Server{Addr: *listen, Handler: (preview.Store{Root: *root, PreviewPort: *previewPort}).StatusHandler(&http.Client{Timeout: 2 * time.Second}), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
