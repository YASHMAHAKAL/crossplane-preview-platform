package preview

import (
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
	crdBytes, err := os.ReadFile("../../../backstage/templates/request-preview/cluster/deploy/cluster/incident-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	crd := string(crdBytes)
	cases := []struct {
		name                string
		change              func(*Snapshot)
		phase, mode, reason string
	}{
		{"app change", func(*Snapshot) {}, "approved", "namespace", "namespaced-app-change"},
		{"cluster API", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: "deploy/cluster/incident-policy.json", Content: crd})
		}, "approved", "vcluster", "cluster-api-required"},
		{"untrusted fork", func(s *Snapshot) { s.PR.Fork = true }, "rejected", "", "untrusted-pr"},
		{"stale CI", func(s *Snapshot) { s.CI.HeadSHA = strings.Repeat("c", 40) }, "waiting-for-ci", "", "ci-not-current"},
		{"bad image", func(s *Snapshot) { s.CI.ImageDigest = "ghcr.io/demo/app:latest" }, "rejected", "", "invalid-image-digest"},
		{"privileged manifest", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: "deploy/cluster/node.json", Content: `{"apiVersion":"v1","kind":"Node"}`})
		}, "rejected", "", "unsupported-cluster-resource"},
		{"modified CRD schema", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: "deploy/cluster/incident-policy.json", Content: strings.Replace(crd, `"high"`, `"critical"`, 1)})
		}, "rejected", "", "unsupported-cluster-resource"},
		{"workflow edit", func(s *Snapshot) { s.Files = append(s.Files, ChangedFile{Path: ".github/workflows/build.yml"}) }, "rejected", "", "unsupported-change"},
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
	if err != nil || result.Phase != "cleaning" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(xrFile); !os.IsNotExist(err) {
		t.Fatalf("XR remains: %v", err)
	}
	if _, err := store.Reconcile(snapshot, config); err != nil {
		t.Fatal(err)
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
	if err != nil || result.Phase != "expired" || result.ReasonCodes[0] != "ttl-expired" {
		t.Fatalf("expiry: %+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, "previews", name)); !os.IsNotExist(err) {
		t.Fatalf("expired XR remains: %v", err)
	}
	status, err := store.Status(name, nil)
	if err != nil || status.Phase != "expired" || status.URL != "" || status.ExpiresAt == "" {
		t.Fatalf("status: %+v %v", status, err)
	}
}
