package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	CleanupTimeout     = 10 * time.Minute
	CleanupSettleDelay = 30 * time.Second
)

// CleanupObserver checks the actual cluster and ingress after the trusted
// GitOps removal has been pushed. An error is never interpreted as absence.
type CleanupObserver interface {
	Remaining(context.Context, string) ([]string, error)
}

type KubectlCleanupObserver struct {
	Context     string
	PreviewPort int
	Client      *http.Client
}

func (observer KubectlCleanupObserver) get(ctx context.Context, resource, name, namespace string) (bool, error) {
	args := []string{"--context", observer.Context, "get", resource, name, "--ignore-not-found", "-o", "name"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	output, err := exec.CommandContext(ctx, "kubectl", args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("kubectl get %s/%s: %w: %s", resource, name, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) != "", nil
}

func (observer KubectlCleanupObserver) Remaining(ctx context.Context, name string) ([]string, error) {
	if !previewPattern.MatchString(name) || len(name) > 63 || observer.Context == "" || observer.PreviewPort < 1 || observer.PreviewPort > 65535 {
		return nil, errors.New("invalid cleanup observer configuration")
	}
	var remaining []string
	for _, resource := range []struct{ kind, item, namespace string }{
		{"applications.argoproj.io", "preview-" + name, "argocd"},
		{"previewenvironments.preview.platform.example.org", name, ""},
		{"namespaces", name, ""},
	} {
		present, err := observer.get(ctx, resource.kind, resource.item, resource.namespace)
		if err != nil {
			return nil, err
		}
		if present {
			remaining = append(remaining, resource.kind+"/"+resource.item)
		}
	}
	output, err := exec.CommandContext(ctx, "kubectl", "--context", observer.Context, "get", "persistentvolumes", "-o", "json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get persistentvolumes: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var volumes struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				ClaimRef struct {
					Namespace string `json:"namespace"`
				} `json:"claimRef"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output, &volumes); err != nil {
		return nil, fmt.Errorf("decode persistentvolumes: %w", err)
	}
	for _, volume := range volumes.Items {
		if volume.Spec.ClaimRef.Namespace == name {
			remaining = append(remaining, "persistentvolumes/"+volume.Metadata.Name)
		}
	}
	client := observer.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	url := fmt.Sprintf("http://%s.localhost:%d/healthz", name, observer.PreviewPort)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("probe preview route: %w", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		remaining = append(remaining, fmt.Sprintf("route/%s (HTTP %d)", name, response.StatusCode))
	}
	return remaining, nil
}

// ObserveCleanup advances a closed or merged PR after a successful GitOps
// publish. It retains the first cleanup timestamp across retries and reports
// a timeout while continuing to retry eventual deletion.
func (store Store) ObserveCleanup(name string, remaining []string, observationErr error) (Decision, error) {
	record, err := store.readStatus(name)
	if err != nil {
		return Decision{}, err
	}
	if record.PR.State != "closed" && record.PR.State != "merged" {
		return Decision{}, errors.New("cleanup observation requires a closed PR")
	}
	if record.Decision.Phase != "cleaning" && record.Decision.Phase != "cleanup-failed" && record.Decision.Phase != "deleted" {
		return Decision{}, errors.New("cleanup observation requires a cleanup decision")
	}
	previewDir, err := store.previewDir(name)
	if err != nil {
		return Decision{}, err
	}
	if _, err := os.Stat(previewDir); err == nil {
		remaining = append(remaining, "trusted GitOps preview directory")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Decision{}, err
	}
	if record.CleanupStartedAt == "" {
		record.CleanupStartedAt = record.UpdatedAt // Existing status records predate the explicit field.
	}
	started, err := time.Parse(time.RFC3339Nano, record.CleanupStartedAt)
	if err != nil {
		return Decision{}, fmt.Errorf("invalid cleanup start: %w", err)
	}
	reason := "pr-closed"
	if record.PR.State == "merged" {
		reason = "pr-merged"
	}
	result := record.Decision
	result.ReasonCodes = []string{reason}
	result.Evidence = []string{}
	if observationErr == nil && len(remaining) == 0 && !store.now().Before(started.Add(CleanupSettleDelay)) {
		result.Phase = "deleted"
		result.ReasonCodes = append(result.ReasonCodes, "cleanup-verified")
		result.Evidence = append(result.Evidence, "Argo Application, XR, namespace, persistent volumes, and preview route absent")
	} else {
		if observationErr != nil {
			result.Evidence = append(result.Evidence, observationErr.Error())
		} else if len(remaining) == 0 {
			result.Evidence = append(result.Evidence, "waiting for cleanup settle delay")
		} else {
			result.Evidence = append(result.Evidence, remaining...)
		}
		if !store.now().Before(started.Add(CleanupTimeout)) {
			result.Phase = "cleanup-failed"
			result.ReasonCodes = append(result.ReasonCodes, "cleanup-timeout")
		} else {
			result.Phase = "cleaning"
		}
	}
	if result.Phase == record.Decision.Phase && strings.Join(result.ReasonCodes, "\x00") == strings.Join(record.Decision.ReasonCodes, "\x00") && strings.Join(result.Evidence, "\x00") == strings.Join(record.Decision.Evidence, "\x00") {
		return result, nil
	}
	record.Decision = result
	record.UpdatedAt = store.now().Format(time.RFC3339Nano)
	return result, atomicJSON(filepath.Join(store.Root, "status", name+".json"), record)
}
