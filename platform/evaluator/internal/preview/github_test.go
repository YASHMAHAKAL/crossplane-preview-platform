package preview

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func zipRecord(t *testing.T, record any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.Create("preview-ci.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(record); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestGitHubSnapshotBindsPRHeadToArtifact(t *testing.T) {
	sha := strings.Repeat("a", 40)
	digest := "ghcr.io/demo/incident-tracker@sha256:" + strings.Repeat("b", 64)
	artifact := zipRecord(t, map[string]string{"headSHA": sha, "imageDigest": digest})
	deployment := checkedInDeploymentConfig(t)
	baselineCRD, err := os.ReadFile("../../../../" + IncidentPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	criticalCRD := withCriticalSeverity(t, string(baselineCRD))
	calls := []string{}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		var body []byte
		switch req.URL.Path {
		case "/repos/demo/incident-tracker/pulls/42":
			body = []byte(`{"number":42,"state":"open","user":{"login":"demo"},"head":{"sha":"` + sha + `","repo":{"full_name":"demo/incident-tracker"}}}`)
		case "/repos/demo/incident-tracker/pulls/42/files":
			body = []byte(`[{"filename":"app/incident-tracker/src/server.js","status":"modified"},{"filename":"deploy/incident-tracker/preview.json","status":"modified"},{"filename":"deploy/cluster/incident-policy.json","status":"modified"}]`)
		case "/repos/demo/incident-tracker/contents/deploy/incident-tracker/preview.json":
			if req.URL.Query().Get("ref") != sha {
				t.Fatalf("deployment content was not fetched from PR head: %s", req.URL)
			}
			body = []byte(`{"encoding":"base64","content":"` + base64.StdEncoding.EncodeToString([]byte(deployment)) + `"}`)
		case "/repos/demo/incident-tracker/contents/deploy/cluster/incident-policy.json":
			if req.URL.Query().Get("ref") != sha {
				t.Fatalf("CRD content was not fetched from PR head: %s", req.URL)
			}
			body = []byte(`{"encoding":"base64","content":"` + base64.StdEncoding.EncodeToString([]byte(criticalCRD)) + `"}`)
		case "/repos/demo/incident-tracker/actions/runs":
			body = []byte(`{"workflow_runs":[{"id":19,"name":"Preview image","event":"push","head_sha":"` + sha + `","status":"completed","conclusion":"success"}]}`)
		case "/repos/demo/incident-tracker/actions/runs/19/artifacts":
			body = []byte(`{"artifacts":[{"id":23,"name":"preview-ci-` + sha + `","expired":false}]}`)
		case "/repos/demo/incident-tracker/actions/artifacts/23/zip":
			body = artifact
		default:
			t.Fatalf("unexpected GitHub path %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	_, config := fixture()
	got, err := (GitHubReader{Client: client, APIBase: "https://api.github.test", Token: "test"}).Snapshot(context.Background(), config, 42, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Request.Size != "small" || got.Request.TTLMinutes != 120 || got.CI.ImageDigest != digest || got.CI.HeadSHA != sha {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	if len(got.Files) != 3 || got.Files[1].Content != deployment || got.Files[1].Status != "modified" || got.Files[2].Content != criticalCRD {
		t.Fatalf("changed deployment content missing: %+v", got.Files)
	}
	if len(calls) != 7 {
		t.Fatalf("expected seven GitHub reads with no request-file lookup, got %v", calls)
	}
}

func TestGitHubSnapshotReadsCandidateCompositionAtExactHead(t *testing.T) {
	sha := strings.Repeat("a", 40)
	composition, _ := candidateCompositionFixture(t)
	content := composition.Files[0].Content
	image := "ghcr.io/demo/incident-tracker@sha256:" + strings.Repeat("b", 64)
	packageDigest := "ghcr.io/demo/function-preview-resources@sha256:" + strings.Repeat("c", 64)
	candidate := CandidateRecord{HeadSHA: sha, PackageDigest: packageDigest,
		PackageTag: "ghcr.io/demo/function-preview-resources:v0.0.0-sha-" + sha + "-run-20-attempt-1", WorkflowRunID: 20}
	imageArtifact := zipRecord(t, map[string]string{"headSHA": sha, "imageDigest": image})
	candidateArtifactData := candidateArtifact(t, candidate)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch req.URL.Path {
		case "/repos/demo/incident-tracker/pulls/42":
			body = []byte(`{"number":42,"state":"open","user":{"login":"demo"},"head":{"sha":"` + sha + `","ref":"candidate","repo":{"full_name":"demo/incident-tracker"}}}`)
		case "/repos/demo/incident-tracker/pulls/42/files":
			body = []byte(`[{"filename":"` + NamespaceCompositionPath + `","status":"modified"}]`)
		case "/repos/demo/incident-tracker/contents/" + NamespaceCompositionPath:
			if req.URL.Query().Get("ref") != sha {
				t.Fatalf("Composition content read outside current PR head: %s", req.URL)
			}
			body = []byte(`{"encoding":"base64","content":"` + base64.StdEncoding.EncodeToString([]byte(content)) + `"}`)
		case "/repos/demo/incident-tracker/actions/runs":
			body = []byte(`{"workflow_runs":[{"id":20,"name":"Preview Crossplane candidate","event":"push","head_sha":"` + sha + `","status":"completed","conclusion":"success"},{"id":19,"name":"Preview image","event":"push","head_sha":"` + sha + `","status":"completed","conclusion":"success"}]}`)
		case "/repos/demo/incident-tracker/actions/runs/19/artifacts":
			body = []byte(`{"artifacts":[{"id":23,"name":"preview-ci-` + sha + `","expired":false}]}`)
		case "/repos/demo/incident-tracker/actions/artifacts/23/zip":
			body = imageArtifact
		case "/repos/demo/incident-tracker/actions/runs/20/artifacts":
			body = []byte(`{"artifacts":[{"id":24,"name":"crossplane-candidate-ci-` + sha + `","expired":false}]}`)
		case "/repos/demo/incident-tracker/actions/artifacts/24/zip":
			body = candidateArtifactData
		default:
			t.Fatalf("unexpected GitHub request %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	_, config := fixture()
	snapshot, err := (GitHubReader{Client: client, APIBase: "https://api.github.test"}).Snapshot(context.Background(), config, 42, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := Evaluate(snapshot, config)
	if result.Phase != "approved" || result.ReasonCodes[0] != "crossplane-composition-change" || result.NamespacePodBudget == nil || *result.NamespacePodBudget != 5 {
		t.Fatalf("exact-head candidate Composition not approved: %+v", result)
	}
}
