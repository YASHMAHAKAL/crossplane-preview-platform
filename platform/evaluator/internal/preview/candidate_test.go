package preview

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func candidateArtifact(t *testing.T, record CandidateRecord) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.Create("crossplane-candidate-ci.json")
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

func TestCandidateForPRRequiresCurrentSuccessfulTrustedRun(t *testing.T) {
	sha := strings.Repeat("a", 40)
	digest := "ghcr.io/demo/function-preview-resources@sha256:" + strings.Repeat("b", 64)
	tag := "ghcr.io/demo/function-preview-resources:v0.0.0-sha-" + sha + "-run-19-attempt-1"
	valid := CandidateRecord{HeadSHA: sha, PackageDigest: digest, PackageTag: tag, WorkflowRunID: 19}
	cases := []struct {
		name       string
		author     string
		status     string
		conclusion string
		record     CandidateRecord
		wantError  bool
		wantKind   error
	}{
		{name: "valid", author: "demo", status: "completed", conclusion: "success", record: valid},
		{name: "untrusted author", author: "stranger", status: "completed", conclusion: "success", record: valid, wantError: true},
		{name: "pending current run", author: "demo", status: "in_progress", record: valid, wantError: true, wantKind: ErrCandidatePending},
		{name: "failed current run", author: "demo", status: "completed", conclusion: "failure", record: valid, wantError: true, wantKind: ErrCandidateFailed},
		{name: "stale artifact head", author: "demo", status: "completed", conclusion: "success", record: CandidateRecord{HeadSHA: strings.Repeat("c", 40), PackageDigest: digest, PackageTag: tag, WorkflowRunID: 19}, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := candidateArtifact(t, tc.record)
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				var body []byte
				switch req.URL.Path {
				case "/repos/demo/project/pulls/42":
					body = []byte(`{"state":"open","user":{"login":"` + tc.author + `"},"head":{"sha":"` + sha + `","ref":"candidate","repo":{"full_name":"demo/project"}}}`)
				case "/repos/demo/project/actions/runs":
					if req.URL.Query().Get("head_sha") != sha {
						t.Fatalf("run lookup was not bound to the PR head: %s", req.URL)
					}
					body = []byte(`{"workflow_runs":[{"id":19,"name":"Preview Crossplane candidate","event":"push","head_sha":"` + sha + `","status":"` + tc.status + `","conclusion":"` + tc.conclusion + `"}]}`)
				case "/repos/demo/project/actions/runs/19/artifacts":
					body = []byte(`{"artifacts":[{"id":23,"name":"crossplane-candidate-ci-` + sha + `","expired":false}]}`)
				case "/repos/demo/project/actions/artifacts/23/zip":
					body = archive
				default:
					t.Fatalf("unexpected GitHub request %s", req.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
			})}
			reader := GitHubReader{Client: client, APIBase: "https://api.github.test"}
			config := Config{Repository: "demo/project", TrustedAuthors: []string{"demo"}}
			got, err := reader.CandidateForPR(context.Background(), config, 42)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected rejection, got %+v", got)
				}
				if tc.wantKind != nil && !errors.Is(err, tc.wantKind) {
					t.Fatalf("wanted %v, got %v", tc.wantKind, err)
				}
				return
			}
			if err != nil || got.PackageDigest != digest || got.Branch != "candidate" {
				t.Fatalf("unexpected candidate %+v, %v", got, err)
			}
		})
	}
}
