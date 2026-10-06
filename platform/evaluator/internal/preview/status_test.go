package preview

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusOnlySurfacesURLAfterHealth(t *testing.T) {
	snapshot, config := fixture()
	store := Store{Root: t.TempDir(), PreviewPort: 8088}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	unhealthy := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	status, err := store.Status(name, unhealthy)
	if err != nil || status.Phase != "provisioning" || status.URL != "" {
		t.Fatalf("got %+v, %v", status, err)
	}
	healthy := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != name+".localhost:8088" || req.URL.Path != "/healthz" {
			t.Fatalf("unexpected probe: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	request := httptest.NewRequest("GET", "/api/previews/"+name, nil)
	response := httptest.NewRecorder()
	store.StatusHandler(healthy).ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"phase":"ready"`) || !strings.Contains(response.Body.String(), name+".localhost:8088") {
		t.Fatalf("status response: %d %s", response.Code, response.Body.String())
	}
}
