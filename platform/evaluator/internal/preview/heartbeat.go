package preview

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// WatcherHeartbeat lives in the operator's local state directory, never in
// the trusted GitOps checkout that Argo CD watches.
type WatcherHeartbeat struct {
	LastAttemptAt       string `json:"lastAttemptAt"`
	LastCompletedAt     string `json:"lastCompletedAt,omitempty"`
	LastSuccessAt       string `json:"lastSuccessAt,omitempty"`
	LastError           string `json:"lastError,omitempty"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`
}

func ReadWatcherHeartbeat(path string) (WatcherHeartbeat, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WatcherHeartbeat{}, err
	}
	var heartbeat WatcherHeartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		return WatcherHeartbeat{}, fmt.Errorf("decode watcher heartbeat: %w", err)
	}
	return heartbeat, nil
}

func WriteWatcherHeartbeat(path string, heartbeat WatcherHeartbeat) error {
	if path == "" {
		return errors.New("watcher heartbeat path is empty")
	}
	return atomicJSON(path, heartbeat)
}

func (heartbeat WatcherHeartbeat) Healthy(now time.Time, maxAge time.Duration) bool {
	if maxAge <= 0 || heartbeat.LastError != "" || heartbeat.ConsecutiveFailures != 0 {
		return false
	}
	attempt, attemptErr := time.Parse(time.RFC3339Nano, heartbeat.LastAttemptAt)
	success, successErr := time.Parse(time.RFC3339Nano, heartbeat.LastSuccessAt)
	if attemptErr != nil || successErr != nil || attempt.After(now) || success.After(now) {
		return false
	}
	return now.Sub(attempt) <= maxAge && now.Sub(success) <= maxAge
}

func WatcherHealthHandler(path string, maxAge time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		heartbeat, err := ReadWatcherHeartbeat(path)
		response := struct {
			Healthy bool `json:"healthy"`
			WatcherHeartbeat
		}{Healthy: err == nil && heartbeat.Healthy(time.Now().UTC(), maxAge), WatcherHeartbeat: heartbeat}
		if err != nil {
			response.LastError = "watcher heartbeat unavailable: " + strings.TrimSpace(err.Error())
		}
		if !response.Healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(response)
	})
}
