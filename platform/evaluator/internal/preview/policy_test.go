package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishedCapabilityDigest(t *testing.T) {
	configBytes, err := os.ReadFile("../../config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := json.Unmarshal(configBytes, &config); err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := os.ReadFile("../../../backstage/templates/request-preview/cluster/deploy/cluster/incident-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(manifest)
	hash := sha256.Sum256(canonical)
	if config.AllowedCRD.ManifestSHA256 != hex.EncodeToString(hash[:]) {
		t.Fatal("config.example.json capability digest differs from Backstage template")
	}
	sourceBytes, err := os.ReadFile("../../../../" + IncidentPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseIncidentPolicy(string(sourceBytes), config.AllowedCRD); err != nil {
		t.Fatalf("checked-in source CRD is outside the supported schema contract: %v", err)
	}
}

func fixture() (Snapshot, Config) {
	config := Config{Service: "incident-tracker", Repository: "demo/incident-tracker",
		TrustedAuthors: []string{"demo"}, Sizes: map[string]Size{"small": {CPU: "250m", Memory: "256Mi"}},
		MaxTTLMinutes: 240, MaxActivePreviews: 3,
		AllowedCRD: AllowedCRD{Name: "incidentpolicies.incidents.demo.local", Group: "incidents.demo.local", Kind: "IncidentPolicy",
			ManifestSHA256: "7d532ba3e0ff33466e650d04a6ce019e2249ce617da4f59364f7a5c5e4be5fea"}}
	sha := strings.Repeat("a", 40)
	snapshot := Snapshot{
		PR:      PullRequest{Number: 42, HeadSHA: sha, Repository: config.Repository, Author: "demo", State: "open"},
		Request: Request{Size: "small", TTLMinutes: 60},
		CI:      CI{State: "success", HeadSHA: sha, ImageDigest: "ghcr.io/demo/incident-tracker@sha256:" + strings.Repeat("b", 64)},
		Files:   []ChangedFile{{Path: "app/incident-tracker/src/server.js"}},
	}
	return snapshot, config
}

func TestPolicyTable(t *testing.T) {
	crdBytes, err := os.ReadFile("../../../../deploy/cluster/incident-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	crd := string(crdBytes)
	criticalCRD := withCriticalSeverity(t, crd)
	deployment := strings.Replace(deploymentFixture, `"replicas":1`, `"replicas":2`, 1)
	cases := []struct {
		name                string
		change              func(*Snapshot)
		phase, mode, reason string
	}{
		{"app change", func(*Snapshot) {}, "approved", "namespace", "namespaced-app-change"},
		{"docs only", func(s *Snapshot) { s.Files = []ChangedFile{{Path: "README.md"}, {Path: "docs/demo.md"}} }, "skipped", "", "no-previewable-change"},
		{"legacy request only", func(s *Snapshot) { s.Files = []ChangedFile{{Path: "preview.request.json"}} }, "skipped", "", "no-previewable-change"},
		{"no changed files", func(s *Snapshot) { s.Files = []ChangedFile{} }, "skipped", "", "no-previewable-change"},
		{"unknown source", func(s *Snapshot) { s.Files = []ChangedFile{{Path: "other-app/index.html"}} }, "rejected", "", "unsupported-change"},
		{"cluster API", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: IncidentPolicyPath, Content: crd})
		}, "approved", "vcluster", "cluster-api-required"},
		{"cluster API schema edit", func(s *Snapshot) {
			s.Files = []ChangedFile{{Path: IncidentPolicyPath, Content: criticalCRD}}
		}, "approved", "vcluster", "cluster-api-required"},
		{"bounded deployment settings", func(s *Snapshot) {
			s.Files = []ChangedFile{{Path: DeploymentConfigPath, Content: deployment}}
		}, "approved", "namespace", "namespaced-deployment-change"},
		{"app and deployment settings", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: DeploymentConfigPath, Content: deployment})
		}, "approved", "namespace", "namespaced-deployment-change"},
		{"cluster API and deployment settings", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: DeploymentConfigPath, Content: deployment}, ChangedFile{Path: IncidentPolicyPath, Content: criticalCRD})
		}, "approved", "vcluster", "cluster-api-required"},
		{"external service type", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: DeploymentConfigPath, Content: strings.Replace(deployment, `"replicas":2`, `"replicas":2,"serviceType":"NodePort"`, 1)})
		}, "rejected", "", "invalid-deployment-config"},
		{"invalid deployment config", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: DeploymentConfigPath, Content: strings.Replace(deployment, `"replicas":2`, `"replicas":4`, 1)})
		}, "rejected", "", "invalid-deployment-config"},
		{"deleted deployment config", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: DeploymentConfigPath, Status: "removed"})
		}, "rejected", "", "invalid-deployment-config"},
		{"untrusted fork", func(s *Snapshot) { s.PR.Fork = true }, "rejected", "", "untrusted-pr"},
		{"stale CI", func(s *Snapshot) { s.CI.HeadSHA = strings.Repeat("c", 40) }, "waiting-for-ci", "", "ci-not-current"},
		{"failed CI", func(s *Snapshot) { s.CI.State = "failure" }, "rejected", "", "ci-failed"},
		{"bad image", func(s *Snapshot) { s.CI.ImageDigest = "ghcr.io/demo/app:latest" }, "rejected", "", "invalid-image-digest"},
		{"privileged manifest", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: "deploy/cluster/node.json", Content: `{"apiVersion":"v1","kind":"Node"}`})
		}, "rejected", "", "unsupported-cluster-resource"},
		{"modified CRD schema", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: IncidentPolicyPath, Content: strings.Replace(crd, `"high"`, `"critical"`, 1)})
		}, "rejected", "", "unsupported-cluster-resource"},
		{"workflow edit", func(s *Snapshot) { s.Files = append(s.Files, ChangedFile{Path: ".github/workflows/build.yml"}) }, "rejected", "", "unsupported-change"},
		{"platform edit", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: "platform/crossplane/function/resources.go"})
		}, "rejected", "", "unsupported-change"},
		{"quota", func(s *Snapshot) { s.ActivePreviews = 3 }, "rejected", "", "capacity-exceeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, config := fixture()
			tc.change(&snapshot)
			got := Evaluate(snapshot, config)
			if got.Phase != tc.phase || got.Mode != tc.mode || got.ReasonCodes[0] != tc.reason {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestReconcileLifecycle(t *testing.T) {
	snapshot, config := fixture()
	store := Store{Root: t.TempDir()}
	result, err := store.Reconcile(snapshot, config)
	if err != nil || result.Mode != "namespace" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	xrFile := filepath.Join(store.Root, "previews", name, "previewenvironment.json")
	if _, err := os.Stat(filepath.Join(store.Root, "previews", name, installerRoleFile)); !os.IsNotExist(err) {
		t.Fatalf("namespace preview received installer grant: %v", err)
	}
	data, err := os.ReadFile(xrFile)
	if err != nil {
		t.Fatal(err)
	}
	var xr map[string]any
	if err := json.Unmarshal(data, &xr); err != nil {
		t.Fatal(err)
	}
	spec := xr["spec"].(map[string]any)
	if spec["image"].(map[string]any)["digest"] != snapshot.CI.ImageDigest {
		t.Fatal("wrong image")
	}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	snapshot.PR.State = "merged"
	result, err = store.Reconcile(snapshot, config)
	if err != nil || result.Phase != "cleaning" || result.Mode != "namespace" || result.ReasonCodes[0] != "pr-merged" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(xrFile); !os.IsNotExist(err) {
		t.Fatalf("XR remains: %v", err)
	}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	result, err = store.ObserveCleanup(name, nil, nil)
	if err != nil || result.Phase != "cleaning" {
		t.Fatalf("settling cleanup: %+v %v", result, err)
	}
	store.Now = func() time.Time { return time.Now().Add(CleanupSettleDelay) }
	result, err = store.ObserveCleanup(name, nil, nil)
	if err != nil || result.Phase != "deleted" || result.ReasonCodes[0] != "pr-merged" {
		t.Fatalf("verified cleanup: %+v %v", result, err)
	}
	result, err = store.Reconcile(snapshot, config)
	if err != nil || result.Phase != "deleted" {
		t.Fatalf("replay: %+v %v", result, err)
	}
	statusFile := filepath.Join(store.Root, "status", name+".json")
	before, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ObserveCleanup(name, nil, nil); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(statusFile)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("cleanup replay changed status: %v", err)
	}
}

func TestCleanupObservationAndTimeout(t *testing.T) {
	snapshot, config := fixture()
	start := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	current := start
	store := Store{Root: t.TempDir(), Now: func() time.Time { return current }}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	snapshot.PR.State = "closed"
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	current = start.Add(9 * time.Minute)
	result, err := store.ObserveCleanup(name, []string{"namespaces/" + name}, nil)
	if err != nil || result.Phase != "cleaning" || result.Evidence[0] != "namespaces/"+name {
		t.Fatalf("pending cleanup: %+v %v", result, err)
	}
	statusFile := filepath.Join(store.Root, "status", name+".json")
	before, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(statusFile)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("cleaning replay changed evidence or timestamp: %v", err)
	}
	current = start.Add(CleanupTimeout)
	result, err = store.ObserveCleanup(name, nil, os.ErrPermission)
	if err != nil || result.Phase != "cleanup-failed" || result.ReasonCodes[1] != "cleanup-timeout" {
		t.Fatalf("failed observation: %+v %v", result, err)
	}
	result, err = store.Reconcile(snapshot, config)
	if err != nil || result.Phase != "cleanup-failed" {
		t.Fatalf("failure replay: %+v %v", result, err)
	}
	current = current.Add(time.Minute)
	result, err = store.ObserveCleanup(name, nil, nil)
	if err != nil || result.Phase != "deleted" || result.ReasonCodes[0] != "pr-closed" {
		t.Fatalf("recovered cleanup: %+v %v", result, err)
	}
	status, err := store.Status(name, nil)
	if err != nil || status.Phase != "deleted" || status.URL != "" {
		t.Fatalf("published status: %+v %v", status, err)
	}
	// A reopened PR must no longer inherit the terminal cleanup state.
	snapshot.PR.State = "open"
	result, err = store.Reconcile(snapshot, config)
	if err != nil || result.Phase != "approved" {
		t.Fatalf("reopened PR: %+v %v", result, err)
	}
}

func TestPreviewExpiresFromFirstApproval(t *testing.T) {
	snapshot, config := fixture()
	start := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	current := start
	store := Store{Root: t.TempDir(), Now: func() time.Time { return current }}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	current = start.Add(59 * time.Minute)
	if result, err := store.Reconcile(snapshot, config); err != nil || result.Phase != "approved" {
		t.Fatalf("early expiry: %+v %v", result, err)
	}
	current = start.Add(60 * time.Minute)
	result, err := store.Reconcile(snapshot, config)
	if err != nil || result.Phase != "cleaning" || result.ReasonCodes[0] != "ttl-expired" {
		t.Fatalf("expiry: %+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, "previews", name)); !os.IsNotExist(err) {
		t.Fatalf("expired XR remains: %v", err)
	}
	status, err := store.Status(name, nil)
	if err != nil || status.Phase != "cleaning" || status.URL != "" || status.ExpiresAt == "" {
		t.Fatalf("status: %+v %v", status, err)
	}
	result, err = store.ObserveCleanup(name, []string{"namespaces/" + name}, nil)
	if err != nil || result.Phase != "cleaning" || result.Evidence[0] != "namespaces/"+name {
		t.Fatalf("expiry cleanup evidence: %+v %v", result, err)
	}
	before, err := os.ReadFile(filepath.Join(store.Root, "status", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err = store.Reconcile(snapshot, config)
	if err != nil || result.Phase != "cleaning" {
		t.Fatalf("expiry replay: %+v %v", result, err)
	}
	after, err := os.ReadFile(filepath.Join(store.Root, "status", name+".json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("expiry replay changed status: %v", err)
	}
	current = current.Add(CleanupSettleDelay)
	result, err = store.ObserveCleanup(name, nil, nil)
	if err != nil || result.Phase != "deleted" || result.ReasonCodes[0] != "ttl-expired" || result.ReasonCodes[1] != "cleanup-verified" {
		t.Fatalf("verified expiry cleanup: %+v %v", result, err)
	}
	result, err = store.Reconcile(snapshot, config)
	if err != nil || result.Phase != "deleted" {
		t.Fatalf("terminal expiry replay: %+v %v", result, err)
	}
}

func TestReconcileUpdatedHeadAndLifetime(t *testing.T) {
	snapshot, config := fixture()
	config.Sizes["medium"] = Size{CPU: "500m", Memory: "512Mi"}
	start := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	current := start
	store := Store{Root: t.TempDir(), Now: func() time.Time { return current }}
	if result, err := store.Reconcile(snapshot, config); err != nil || result.Phase != "approved" {
		t.Fatalf("initial approval: %+v %v", result, err)
	}
	name, _ := PreviewName(config.Service, snapshot.PR.Number)
	xrFile := filepath.Join(store.Root, "previews", name, "previewenvironment.json")
	statusFile := filepath.Join(store.Root, "status", name+".json")

	current = start.Add(10 * time.Minute)
	snapshot.PR.HeadSHA = strings.Repeat("c", 40)
	snapshot.Request = Request{Size: "medium", TTLMinutes: 90}
	if result, err := store.Reconcile(snapshot, config); err != nil || result.Phase != "waiting-for-ci" {
		t.Fatalf("stale CI must wait: %+v %v", result, err)
	}
	if _, err := os.Stat(xrFile); !os.IsNotExist(err) {
		t.Fatalf("old head XR remains: %v", err)
	}

	snapshot.CI.HeadSHA = snapshot.PR.HeadSHA
	snapshot.CI.ImageDigest = "ghcr.io/demo/incident-tracker@sha256:" + strings.Repeat("d", 64)
	if result, err := store.Reconcile(snapshot, config); err != nil || result.Phase != "approved" {
		t.Fatalf("updated approval: %+v %v", result, err)
	}
	statusBytes, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		FirstApprovedAt string `json:"firstApprovedAt"`
		ExpiresAt       string `json:"expiresAt"`
	}
	if err := json.Unmarshal(statusBytes, &status); err != nil {
		t.Fatal(err)
	}
	if status.FirstApprovedAt != start.Format(time.RFC3339Nano) || status.ExpiresAt != start.Add(90*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("updated lifetime: %+v", status)
	}
	xrBytes, err := os.ReadFile(xrFile)
	if err != nil {
		t.Fatal(err)
	}
	var xr struct {
		Spec struct {
			PR    PullRequest `json:"pr"`
			Image struct {
				Digest string `json:"digest"`
			} `json:"image"`
			Request Request `json:"request"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(xrBytes, &xr); err != nil {
		t.Fatal(err)
	}
	if xr.Spec.PR.HeadSHA != snapshot.PR.HeadSHA || xr.Spec.Image.Digest != snapshot.CI.ImageDigest || xr.Spec.Request != snapshot.Request {
		t.Fatalf("updated XR mismatch: %+v", xr.Spec)
	}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
	}
	replayedStatus, err := os.ReadFile(statusFile)
	if err != nil || !bytes.Equal(statusBytes, replayedStatus) {
		t.Fatalf("replay changed status: %v", err)
	}
	current = start.Add(90 * time.Minute)
	if result, err := store.Reconcile(snapshot, config); err != nil || result.Phase != "cleaning" {
		t.Fatalf("updated expiry: %+v %v", result, err)
	}
}
