package preview

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func checkedInDeploymentConfig(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../../../../" + DeploymentConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestCheckedInDeploymentConfig(t *testing.T) {
	spec, err := ParseDeploymentConfig(checkedInDeploymentConfig(t))
	if err != nil || spec.Replicas != 1 || spec.Resources.Requests.CPU != "100m" {
		t.Fatalf("checked-in deployment config: %+v %v", spec, err)
	}
}

func TestDeploymentConfigRejectsUnsupportedValues(t *testing.T) {
	base := checkedInDeploymentConfig(t)
	cases := map[string]string{
		"too many replicas":    strings.Replace(base, `"replicas": 1`, `"replicas": 3`, 1),
		"unknown service type": strings.Replace(base, `"replicas": 1`, `"replicas": 1, "serviceType": "LoadBalancer"`, 1),
		"excessive limit":      strings.Replace(base, `"memory": "512Mi"`, `"memory": "4096Mi"`, 1),
		"missing resources":    strings.Replace(base, `"requests"`, `"missing"`, 1),
		"trailing document":    base + `{}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDeploymentConfig(content); err == nil {
				t.Fatal("unsupported deployment config accepted")
			}
		})
	}
}

func TestDeploymentChangeProducesBoundedVClusterXR(t *testing.T) {
	snapshot, config := fixture()
	changed := strings.Replace(checkedInDeploymentConfig(t), `"replicas": 1`, `"replicas": 2`, 1)
	snapshot.Files = []ChangedFile{{Path: DeploymentConfigPath, Status: "modified", Content: changed}}
	decision := Evaluate(snapshot, config)
	if decision.Phase != "approved" || decision.Mode != "vcluster" || decision.ReasonCodes[0] != "deployment-stack-change" ||
		decision.Deployment == nil || decision.Deployment.Replicas != 2 {
		t.Fatalf("deployment decision: %+v", decision)
	}
	xr, err := MakeXR(snapshot, decision, config)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(xr)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Spec struct {
			Deployment DeploymentSpec `json:"deployment"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.Spec.Deployment.Replicas != 2 || result.Spec.Deployment.Resources.Limits.Memory != "512Mi" ||
		strings.Contains(string(encoded), "IncidentTrackerDeployment") {
		t.Fatalf("XR does not contain only normalized settings: %s", encoded)
	}
}
