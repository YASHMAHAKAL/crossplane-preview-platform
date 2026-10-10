package preview

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type readinessFunc func(context.Context, string, string) (bool, error)

func (fn readinessFunc) Ready(ctx context.Context, name, sha string) (bool, error) {
	return fn(ctx, name, sha)
}

func TestStatusOnlySurfacesURLAfterHealth(t *testing.T) {
	snapshot, config := fixture()
	store := Store{Root: t.TempDir(), PreviewPort: 8088, Readiness: readinessFunc(func(context.Context, string, string) (bool, error) { return true, nil })}
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
	store.Readiness = readinessFunc(func(context.Context, string, string) (bool, error) { return false, nil })
	status, err = store.Status(name, healthy)
	if err != nil || status.Phase != "provisioning" || status.URL != "" {
		t.Fatalf("healthy route with unready XR: %+v, %v", status, err)
	}
	store.Readiness = readinessFunc(func(context.Context, string, string) (bool, error) { return true, nil })
	request := httptest.NewRequest("GET", "/api/previews/"+name, nil)
	response := httptest.NewRecorder()
	store.StatusHandler(healthy).ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"phase":"ready"`) || !strings.Contains(response.Body.String(), name+".localhost:8088") {
		t.Fatalf("status response: %d %s", response.Code, response.Body.String())
	}
	store.Readiness = readinessFunc(func(context.Context, string, string) (bool, error) { return false, errors.New("cluster unavailable") })
	status, err = store.Status(name, healthy)
	if err != nil || status.Phase != "degraded" || status.URL != "" || !strings.Contains(strings.Join(status.Evidence, " "), "cluster unavailable") {
		t.Fatalf("readiness error must be visible without URL: %+v, %v", status, err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, status.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return deadline }
	status, err = store.Status(name, healthy)
	if err != nil || status.Phase != "expired" || status.URL != "" || len(status.ReasonCodes) != 1 || status.ReasonCodes[0] != "ttl-expired" {
		t.Fatalf("expired preview must hide the URL even if its route is live: %+v, %v", status, err)
	}
}

func TestCandidateURLRequiresCurrentVirtualInstall(t *testing.T) {
	snapshot, config := candidateFixture(t)
	store := Store{Root: t.TempDir(), PreviewPort: 8088,
		Readiness: readinessFunc(func(context.Context, string, string) (bool, error) { return true, nil })}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != name+"-c.localhost:8088" {
			t.Fatalf("candidate status probed baseline route: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	status, err := store.Status(name, client)
	if err != nil || status.Phase != "provisioning" || status.URL != "" {
		t.Fatalf("missing candidate install must hide URL: %+v, %v", status, err)
	}
	state := CandidatePreviewState{HeadSHA: snapshot.PR.HeadSHA, PackageDigest: snapshot.Candidate.PackageDigest, State: "ready"}
	if err := store.WriteCandidateState(name, state); err != nil {
		t.Fatal(err)
	}
	status, err = store.Status(name, client)
	if err != nil || status.Phase != "ready" || status.URL != "http://"+name+"-c.localhost:8088" {
		t.Fatalf("candidate URL missing: %+v, %v", status, err)
	}
	snapshot.Candidate.State = "pending"
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	status, err = store.Status(name, client)
	if err != nil || status.Phase != "waiting-for-ci" || status.URL != "" {
		t.Fatalf("new pending candidate must hide old URL: %+v, %v", status, err)
	}
}
