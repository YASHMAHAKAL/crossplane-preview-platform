package preview

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func withCriticalSeverity(t *testing.T, content string) string {
	t.Helper()
	var manifest map[string]any
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		t.Fatal(err)
	}
	severity := manifest["spec"].(map[string]any)["versions"].([]any)[0].(map[string]any)["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)["properties"].(map[string]any)["spec"].(map[string]any)["properties"].(map[string]any)["severity"].(map[string]any)
	severity["enum"] = []any{"low", "high", "critical"}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestIncidentPolicySchemaEditProducesNormalizedXR(t *testing.T) {
	baseline, err := os.ReadFile("../../../../" + IncidentPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, config := fixture()
	snapshot.Files = []ChangedFile{
		{Path: DeploymentConfigPath, Content: strings.Replace(deploymentFixture, `"replicas":1`, `"replicas":2`, 1)},
		{Path: IncidentPolicyPath, Content: withCriticalSeverity(t, string(baseline))},
	}
	result := Evaluate(snapshot, config)
	if result.Phase != "approved" || result.Mode != "vcluster" || result.IncidentPolicy == nil || !result.IncidentPolicy.AllowCritical ||
		result.Deployment == nil || result.Deployment.Replicas != 2 || !strings.Contains(strings.Join(result.Evidence, " "), "allowCritical=true") {
		t.Fatalf("mixed decision: %+v", result)
	}
	xr, err := MakeXR(snapshot, result, config)
	if err != nil {
		t.Fatal(err)
	}
	spec := xr["spec"].(map[string]any)
	if spec["incidentPolicy"].(*IncidentPolicySpec).AllowCritical != true || spec["deployment"].(*DeploymentSpec).Replicas != 2 {
		t.Fatalf("XR lost normalized settings: %#v", spec)
	}
	encoded, err := json.Marshal(xr)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "CustomResourceDefinition") || strings.Contains(string(encoded), "openAPIV3Schema") {
		t.Fatalf("XR includes raw CRD: %s", encoded)
	}
}

func TestIncidentPolicyRejectsOtherSchemaEdits(t *testing.T) {
	baseline, err := os.ReadFile("../../../../" + IncidentPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	_, config := fixture()
	critical := withCriticalSeverity(t, string(baseline))
	cases := map[string]string{
		"changed scope":  strings.Replace(critical, `"scope":"Namespaced"`, `"scope":"Cluster"`, 1),
		"extra severity": strings.Replace(critical, `"critical"]`, `"critical","urgent"]`, 1),
		"changed type":   strings.Replace(critical, `"type":"string"`, `"type":"integer"`, 1),
		"extra metadata": strings.Replace(critical, `"kind":"CustomResourceDefinition"`, `"kind":"CustomResourceDefinition","labels":{"owner":"pr"}`, 1),
		"malformed":      `{"apiVersion":`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseIncidentPolicy(content, config.AllowedCRD); err == nil {
				t.Fatal("unsupported CRD edit accepted")
			}
		})
	}
}
