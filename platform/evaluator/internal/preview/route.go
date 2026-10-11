package preview

import (
	"context"
	"fmt"
	"net/http"
)

// previewRouteRequest connects to the local kind ingress without depending on
// wildcard localhost DNS, IPv6 choice, or proxy environment settings. The Host
// header remains the preview hostname so ingress routing is still exercised.
func previewRouteRequest(ctx context.Context, name string, port int) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/healthz", port), nil)
	if err != nil {
		return nil, err
	}
	request.Host = name + ".localhost"
	return request, nil
}
