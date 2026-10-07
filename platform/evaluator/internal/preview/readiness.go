package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ReadinessChecker verifies that the exact PR head has reconciled before the
// status API exposes a live URL. A healthy route by itself can be stale.
type ReadinessChecker interface {
	Ready(context.Context, string, string) (bool, error)
}

type KubectlReadinessChecker struct {
	Context string
}

func (checker KubectlReadinessChecker) Ready(ctx context.Context, name, headSHA string) (bool, error) {
	if checker.Context == "" || !previewPattern.MatchString(name) || !shaPattern.MatchString(headSHA) {
		return false, errors.New("invalid readiness check configuration")
	}
	output, err := exec.CommandContext(ctx, "kubectl", "--context", checker.Context,
		"get", "previewenvironment", name, "--ignore-not-found", "-o", "json").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("kubectl get previewenvironment/%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	if len(strings.TrimSpace(string(output))) == 0 {
		return false, nil
	}
	return xrReady(output, name, headSHA)
}

func xrReady(data []byte, name, headSHA string) (bool, error) {
	var xr struct {
		Metadata struct {
			Name       string `json:"name"`
			Generation int64  `json:"generation"`
		} `json:"metadata"`
		Spec struct {
			PR struct {
				HeadSHA string `json:"headSHA"`
			} `json:"pr"`
		} `json:"spec"`
		Status struct {
			Conditions []struct {
				Type               string `json:"type"`
				Status             string `json:"status"`
				ObservedGeneration int64  `json:"observedGeneration"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &xr); err != nil {
		return false, fmt.Errorf("decode preview readiness: %w", err)
	}
	if xr.Metadata.Name != name || xr.Metadata.Generation < 1 || xr.Spec.PR.HeadSHA != headSHA {
		return false, nil
	}
	ready, synced := false, false
	for _, condition := range xr.Status.Conditions {
		current := condition.Status == "True" && condition.ObservedGeneration >= xr.Metadata.Generation
		switch condition.Type {
		case "Ready":
			ready = current
		case "Synced":
			synced = current
		}
	}
	return ready && synced, nil
}
