package preview

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestObservePhaseMeasuresFirstReadyPoll(t *testing.T) {
	snapshot, config := fixture()
	baseline := time.Now().UTC().Add(-2 * time.Second)
	store := Store{Root: t.TempDir(), PreviewPort: 8088, Now: func() time.Time { return baseline }, Readiness: readinessFunc(func(context.Context, string, string) (bool, error) { return true, nil })}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	store.Now = nil
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	var probes atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		code := http.StatusServiceUnavailable
		if probes.Add(1) > 1 {
			code = http.StatusOK
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sample, err := store.ObservePhase(ctx, name, "ready", "namespace", time.Second, client)
	if err != nil || sample.Outcome != "observed" || sample.LateStart || sample.DurationMS == nil || *sample.DurationMS < 2000 {
		t.Fatalf("unexpected measurement: %+v, %v", sample, err)
	}
	if len(sample.Events) != 2 || sample.Events[0].Phase != "provisioning" || sample.Events[1].Phase != "ready" {
		t.Fatalf("phase sequence: %+v", sample.Events)
	}
}

func TestObservePhaseMarksLateStartWithoutLatency(t *testing.T) {
	name := "incident-tracker-pr-55"
	store := Store{Root: t.TempDir(), PreviewPort: 8088, Readiness: readinessFunc(func(context.Context, string, string) (bool, error) { return true, nil })}
	stamp := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	record := statusRecord{Name: name, PR: PullRequest{Number: 55, HeadSHA: strings.Repeat("a", 40), State: "closed"},
		Decision:  Decision{Phase: "deleted", Mode: "vcluster", ReasonCodes: []string{"pr-closed", "cleanup-verified"}},
		UpdatedAt: stamp, CleanupStartedAt: stamp}
	if err := atomicJSON(filepath.Join(store.Root, "status", name+".json"), record); err != nil {
		t.Fatal(err)
	}
	sample, err := store.ObservePhase(context.Background(), name, "deleted", "vcluster", time.Second, nil)
	if err != nil || sample.Outcome != "late-start" || !sample.LateStart || sample.DurationMS != nil || sample.BaselineAt == "" {
		t.Fatalf("late start must not claim latency: %+v, %v", sample, err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, "status", name+".json")); err != nil {
		t.Fatal(err)
	}
}

func TestObservePhaseDoesNotClaimReadyForUnhealthyRoute(t *testing.T) {
	snapshot, config := fixture()
	store := Store{Root: t.TempDir(), PreviewPort: 8088, Readiness: readinessFunc(func(context.Context, string, string) (bool, error) { return true, nil })}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 1100*time.Millisecond)
	defer cancel()
	sample, err := store.ObservePhase(ctx, name, "ready", "namespace", time.Second, client)
	if err != context.DeadlineExceeded || sample.Outcome != "timeout" || sample.DurationMS != nil || len(sample.Events) != 1 || sample.Events[0].Phase != "provisioning" {
		t.Fatalf("unhealthy route must remain unready: %+v, %v", sample, err)
	}
}
