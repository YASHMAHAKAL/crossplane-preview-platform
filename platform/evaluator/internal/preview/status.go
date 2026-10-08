package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Status struct {
	Name        string   `json:"name"`
	Phase       string   `json:"phase"`
	Mode        string   `json:"mode,omitempty"`
	ReasonCodes []string `json:"reasonCodes"`
	Evidence    []string `json:"evidence"`
	HeadSHA     string   `json:"headSHA"`
	PRNumber    int      `json:"prNumber"`
	URL         string   `json:"url,omitempty"`
	UpdatedAt   string   `json:"updatedAt"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
}

func (store Store) Status(name string, client *http.Client) (Status, error) {
	if _, err := store.previewDir(name); err != nil {
		return Status{}, err
	}
	data, err := os.ReadFile(filepath.Join(store.Root, "status", name+".json"))
	if err != nil {
		return Status{}, err
	}
	var record struct {
		Name      string      `json:"name"`
		PR        PullRequest `json:"pr"`
		Decision  Decision    `json:"decision"`
		UpdatedAt string      `json:"updatedAt"`
		ExpiresAt string      `json:"expiresAt"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return Status{}, err
	}
	if record.Name != name {
		return Status{}, errors.New("status record name mismatch")
	}
	status := Status{Name: name, Phase: record.Decision.Phase, Mode: record.Decision.Mode,
		ReasonCodes: record.Decision.ReasonCodes, Evidence: record.Decision.Evidence,
		HeadSHA: record.Decision.HeadSHA, PRNumber: record.PR.Number, UpdatedAt: record.UpdatedAt, ExpiresAt: record.ExpiresAt}
	if status.Phase != "approved" {
		return status, nil
	}
	if status.ExpiresAt != "" {
		deadline, err := time.Parse(time.RFC3339Nano, status.ExpiresAt)
		if err != nil {
			return Status{}, fmt.Errorf("invalid saved preview expiry: %w", err)
		}
		if !store.now().Before(deadline) {
			status.Phase = "expired"
			status.ReasonCodes = []string{"ttl-expired"}
			status.Evidence = []string{"preview deadline passed; watcher cleanup pending"}
			return status, nil
		}
	}
	status.Phase = "provisioning"
	if store.Readiness == nil {
		status.Phase = "degraded"
		status.Evidence = append(status.Evidence, "readiness checker is not configured")
		return status, nil
	}
	checkContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	ready, checkErr := store.Readiness.Ready(checkContext, name, status.HeadSHA)
	cancel()
	if checkErr != nil {
		status.Phase = "degraded"
		status.Evidence = append(status.Evidence, checkErr.Error())
		return status, nil
	}
	if !ready {
		return status, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	port := store.PreviewPort
	if port == 0 {
		port = 80
	}
	if port < 1 || port > 65535 {
		return Status{}, errors.New("invalid preview host port")
	}
	url := "http://" + name + ".localhost"
	if port != 80 {
		url += ":" + strconv.Itoa(port)
	}
	response, err := client.Get(url + "/healthz")
	if err == nil {
		response.Body.Close()
		if response.StatusCode == http.StatusOK {
			status.Phase = "ready"
			status.URL = url
		}
	}
	return status, nil
}

func (store Store) StatusHandler(client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/previews/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/previews/")
		status, err := store.Status(name, client)
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("status unavailable: %v", err), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(status)
	})
}
