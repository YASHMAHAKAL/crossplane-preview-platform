package preview

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
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
	request := base64.StdEncoding.EncodeToString([]byte(`{"size":"medium","ttlMinutes":90}`))
	artifact := zipRecord(t, map[string]string{"headSHA": sha, "imageDigest": digest})
	calls := []string{}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		var body []byte
		switch req.URL.Path {
		case "/repos/demo/incident-tracker/pulls/42":
			body = []byte(`{"number":42,"state":"open","user":{"login":"demo"},"head":{"sha":"` + sha + `","repo":{"full_name":"demo/incident-tracker"}}}`)
		case "/repos/demo/incident-tracker/contents/preview.request.json":
			body = []byte(`{"encoding":"base64","content":"` + request + `"}`)
		case "/repos/demo/incident-tracker/pulls/42/files":
			body = []byte(`[{"filename":"app/incident-tracker/src/server.js"}]`)
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
	if got.Request.Size != "medium" || got.Request.TTLMinutes != 90 || got.CI.ImageDigest != digest || got.CI.HeadSHA != sha {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	if len(calls) != 6 {
		t.Fatalf("expected six GitHub reads, got %v", calls)
	}
}
