package preview

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type guestReaderFunc func(context.Context, string, string) (GuestInventory, error)

func (fn guestReaderFunc) Read(ctx context.Context, name, head string) (GuestInventory, error) {
	return fn(ctx, name, head)
}

func TestGuestInventoryOnlyForReadyVCluster(t *testing.T) {
	name := "incident-tracker-pr-42"
	head := strings.Repeat("a", 40)
	store := Store{Root: t.TempDir(), PreviewPort: 8088,
		Readiness: readinessFunc(func(context.Context, string, string) (bool, error) { return true, nil })}
	record := statusRecord{Name: name, PR: PullRequest{Number: 42},
		Decision:  Decision{Phase: "approved", Mode: "vcluster", HeadSHA: head, ReasonCodes: []string{"cluster-api-required"}},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}
	path := filepath.Join(store.Root, "status", name+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func() {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	reads := 0
	reader := guestReaderFunc(func(_ context.Context, gotName, gotHead string) (GuestInventory, error) {
		reads++
		if gotName != name || gotHead != head {
			t.Fatalf("wrong guest request: %s %s", gotName, gotHead)
		}
		return GuestInventory{Name: name, HeadSHA: head, Resources: []GuestResource{}}, nil
	})
	handler := store.GuestInventoryHandler(client, reader)
	request := httptest.NewRequest(http.MethodGet, "/api/previews/"+name+"/guest-resources", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || reads != 1 || !strings.Contains(response.Body.String(), head) {
		t.Fatalf("ready guest inventory: HTTP %d, reads %d, body %s", response.Code, reads, response.Body.String())
	}
	record.Decision.Mode = "namespace"
	write()
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || reads != 1 {
		t.Fatalf("namespace preview exposed guest inventory: HTTP %d, reads %d", response.Code, reads)
	}
	record.Decision.Mode = "vcluster"
	record.Decision.Phase = "cleaning"
	write()
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || reads != 1 {
		t.Fatalf("closed preview exposed guest inventory: HTTP %d, reads %d", response.Code, reads)
	}
}

func TestGuestSummaryOmitsUnapprovedFields(t *testing.T) {
	object := map[string]any{
		"kind":     "Pod",
		"metadata": map[string]any{"name": "incident-tracker-123", "annotations": map[string]any{"secret": "DO-NOT-RETURN"}},
		"spec":     map[string]any{"containers": []any{map[string]any{"image": "ghcr.io/demo/app@sha256:abc", "env": []any{map[string]any{"value": "DO-NOT-RETURN"}}}}},
		"status":   map[string]any{"phase": "Running", "conditions": []any{map[string]any{"type": "Ready", "status": "True"}}},
	}
	resource := guestSummary(object, "Pod")
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	if resource.State != "Ready" || !strings.Contains(string(data), "ghcr.io/demo/app") || strings.Contains(string(data), "DO-NOT-RETURN") {
		t.Fatalf("unsafe or incomplete guest summary: %s", data)
	}
	delete(object, "kind")
	if got := guestSummary(object, "Pod"); got.Kind != "Pod" || got.State != "Ready" {
		t.Fatalf("Kubernetes list item without kind was not classified: %+v", got)
	}
}
