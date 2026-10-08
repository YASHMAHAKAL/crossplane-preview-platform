package preview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherHeartbeatHealthAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watcher-health.json")
	now := time.Now().UTC()
	stamp := now.Add(-10 * time.Second).Format(time.RFC3339Nano)
	heartbeat := WatcherHeartbeat{LastAttemptAt: stamp, LastCompletedAt: stamp, LastSuccessAt: stamp}
	if err := WriteWatcherHeartbeat(path, heartbeat); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	WatcherHealthHandler(path, 3*time.Minute).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("healthy watcher: %d %s", response.Code, response.Body.String())
	}
	heartbeat.LastError = "GitHub unavailable"
	heartbeat.ConsecutiveFailures = 1
	if err := WriteWatcherHeartbeat(path, heartbeat); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	WatcherHealthHandler(path, 3*time.Minute).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed watcher: %d %s", response.Code, response.Body.String())
	}
	var body struct{ Healthy bool }
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Healthy {
		t.Fatalf("unhealthy response: %+v %v", body, err)
	}
	heartbeat.LastError = ""
	heartbeat.ConsecutiveFailures = 0
	heartbeat.LastAttemptAt = now.Add(-4 * time.Minute).Format(time.RFC3339Nano)
	if heartbeat.Healthy(now, 3*time.Minute) {
		t.Fatal("stale attempt reported healthy")
	}
}
