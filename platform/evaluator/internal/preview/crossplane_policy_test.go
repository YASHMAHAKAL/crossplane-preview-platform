package preview

import (
	"os"
	"strings"
	"testing"
)

func candidateFixture(t *testing.T) (Snapshot, Config) {
	t.Helper()
	snapshot, config := fixture()
	contents, err := os.ReadFile("../../../../platform/crossplane/function/resources.go")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Files = []ChangedFile{{Path: "platform/crossplane/function/resources.go", Status: "modified", Content: string(contents)}}
	snapshot.Candidate = CandidateCI{State: "success", HeadSHA: snapshot.PR.HeadSHA,
		PackageDigest: "ghcr.io/demo/function-preview-resources@sha256:" + strings.Repeat("c", 64), WorkflowRunID: 19}
	return snapshot, config
}

func TestCrossplaneFunctionCandidateProducesIsolatedChildXR(t *testing.T) {
	snapshot, config := candidateFixture(t)
	result := Evaluate(snapshot, config)
	if result.Phase != "approved" || result.Mode != "vcluster" || result.ReasonCodes[0] != "crossplane-function-change" || result.Candidate == nil {
		t.Fatalf("candidate decision: %+v", result)
	}
	host, err := MakeXR(snapshot, result, config)
	if err != nil {
		t.Fatal(err)
	}
	annotation := host["metadata"].(map[string]any)["annotations"].(map[string]string)["preview.platform.example.org/candidate-package"]
	if annotation != snapshot.Candidate.PackageDigest {
		t.Fatalf("host GitOps did not retain candidate digest: %s", annotation)
	}
	child, childName, err := MakeCandidateXR(snapshot, result, config)
	if err != nil {
		t.Fatal(err)
	}
	if childName != "incident-tracker-pr-42-c" || child["metadata"].(map[string]any)["name"] != childName {
		t.Fatalf("wrong candidate child name: %s", childName)
	}
	spec := child["spec"].(map[string]any)
	if spec["crossplane"].(map[string]any)["compositionRef"].(map[string]string)["name"] != "preview-namespace" ||
		spec["preview"].(map[string]string)["host"] != childName+".localhost" ||
		spec["image"].(map[string]string)["digest"] != snapshot.CI.ImageDigest {
		t.Fatalf("child XR did not carry the normalized app request: %+v", spec)
	}
}

func TestCrossplaneCandidateFailsClosed(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(*Snapshot)
	}{
		{"pending build", "candidate-ci-not-current", func(s *Snapshot) { s.Candidate.State = "pending" }},
		{"failed build", "candidate-ci-failed", func(s *Snapshot) { s.Candidate.State = "failure" }},
		{"stale build", "candidate-ci-not-current", func(s *Snapshot) { s.Candidate.HeadSHA = strings.Repeat("d", 40) }},
		{"foreign package", "invalid-candidate-package", func(s *Snapshot) {
			s.Candidate.PackageDigest = "ghcr.io/other/function-preview-resources@sha256:" + strings.Repeat("c", 64)
		}},
		{"unparsed Go", "invalid-crossplane-source", func(s *Snapshot) { s.Files[0].Content = "package main\nfunc broken(" }},
		{"deleted source", "unsupported-crossplane-change", func(s *Snapshot) { s.Files[0].Status = "removed" }},
		{"changed Dockerfile", "unsupported-crossplane-change", func(s *Snapshot) {
			s.Files = append(s.Files, ChangedFile{Path: "platform/crossplane/function/Dockerfile"})
		}},
		{"changed XRD", "unsupported-crossplane-change", func(s *Snapshot) { s.Files = append(s.Files, ChangedFile{Path: "platform/crossplane/xrd.yaml"}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, config := candidateFixture(t)
			tc.change(&snapshot)
			result := Evaluate(snapshot, config)
			if result.ReasonCodes[0] != tc.reason || (result.Phase != "rejected" && result.Phase != "waiting-for-ci") {
				t.Fatalf("unexpected candidate decision: %+v", result)
			}
		})
	}
}
