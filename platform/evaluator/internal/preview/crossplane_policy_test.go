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

func candidateCompositionFixture(t *testing.T) (Snapshot, Config) {
	t.Helper()
	snapshot, config := candidateFixture(t)
	baseline, err := os.ReadFile("../../../../platform/crossplane/compositions/namespace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(baseline), "podBudget: 4", "podBudget: 5", 1)
	if strings.Count(changed, "podBudget: 5") != 1 {
		t.Fatal("namespace Composition has no bounded pod budget fixture")
	}
	snapshot.Files = []ChangedFile{{Path: NamespaceCompositionPath, Status: "modified", Content: changed}}
	return snapshot, config
}

func TestNamespaceCompositionCandidateIsNormalized(t *testing.T) {
	snapshot, config := candidateCompositionFixture(t)
	result := Evaluate(snapshot, config)
	if result.Phase != "approved" || result.Mode != "vcluster" || result.ReasonCodes[0] != "crossplane-composition-change" ||
		result.NamespacePodBudget == nil || *result.NamespacePodBudget != 5 || result.Candidate == nil {
		t.Fatalf("bounded Composition decision: %+v", result)
	}
	child, _, err := MakeCandidateXR(snapshot, result, config)
	if err != nil || child["spec"].(map[string]any)["crossplane"].(map[string]any)["compositionRef"].(map[string]string)["name"] != "preview-namespace" {
		t.Fatalf("candidate child XR: %+v %v", child, err)
	}
}

func TestNamespaceCompositionCandidateRejectsOtherChanges(t *testing.T) {
	cases := []struct {
		name, before, after string
	}{
		{"mode", "mode: namespace", "mode: vcluster"},
		{"function", "name: function-preview-resources", "name: arbitrary-function"},
		{"extra field", "podBudget: 5", "podBudget: 5\n        privileged: true"},
		{"too large budget", "podBudget: 5", "podBudget: 6"},
		{"second document", "podBudget: 5", "podBudget: 5\n---\nkind: Secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, config := candidateCompositionFixture(t)
			snapshot.Files[0].Content = strings.Replace(snapshot.Files[0].Content, tc.before, tc.after, 1)
			result := Evaluate(snapshot, config)
			if result.Phase != "rejected" || result.ReasonCodes[0] != "unsupported-crossplane-change" {
				t.Fatalf("unsupported Composition accepted: %+v", result)
			}
		})
	}
}

func candidateXRDFixture(t *testing.T) (Snapshot, Config) {
	t.Helper()
	snapshot, config := candidateFixture(t)
	baseline, err := os.ReadFile("testdata/xrd-baseline.yaml")
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(baseline), reasonCodesBaseline, reasonCodesCandidate, 1)
	if changed == string(baseline) {
		t.Fatal("XRD reasonCodes baseline field is missing")
	}
	snapshot.Files = []ChangedFile{{Path: PreviewXRDPath, Status: "modified", Content: changed}}
	return snapshot, config
}

func TestBoundedXRDCandidate(t *testing.T) {
	snapshot, config := candidateXRDFixture(t)
	result := Evaluate(snapshot, config)
	if result.Phase != "approved" || result.Mode != "vcluster" || result.ReasonCodes[0] != "crossplane-xrd-change" ||
		result.XRDMinReasonCodes == nil || *result.XRDMinReasonCodes != 1 || result.Candidate == nil {
		t.Fatalf("bounded XRD decision: %+v", result)
	}
	child, _, err := MakeCandidateXR(snapshot, result, config)
	if err != nil || child["spec"].(map[string]any)["crossplane"].(map[string]any)["compositionRef"].(map[string]string)["name"] != "preview-namespace" {
		t.Fatalf("candidate child XR: %+v %v", child, err)
	}
}

func TestXRDCandidateRejectsOtherEdits(t *testing.T) {
	cases := []struct{ name, before, after string }{
		{"removed constraint", "minItems: 1", "minItems: 0"},
		{"widened mode", "enum: [namespace, vcluster]", "enum: [namespace, vcluster, privileged]"},
		{"extra document", "minItems: 1", "minItems: 1\n---\nkind: Secret"},
		{"changed validation", "minimum: 1", "minimum: 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, config := candidateXRDFixture(t)
			snapshot.Files[0].Content = strings.Replace(snapshot.Files[0].Content, tc.before, tc.after, 1)
			result := Evaluate(snapshot, config)
			if result.Phase != "rejected" || result.ReasonCodes[0] != "unsupported-crossplane-change" {
				t.Fatalf("unsupported XRD edit accepted: %+v", result)
			}
		})
	}
	for _, status := range []string{"added", "removed"} {
		t.Run(status, func(t *testing.T) {
			snapshot, config := candidateXRDFixture(t)
			snapshot.Files[0].Status = status
			result := Evaluate(snapshot, config)
			if result.Phase != "rejected" || result.ReasonCodes[0] != "unsupported-crossplane-change" {
				t.Fatalf("non-modified XRD accepted: %+v", result)
			}
		})
	}
}
