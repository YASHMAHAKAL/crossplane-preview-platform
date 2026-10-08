package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func sampleXR(mode string) PreviewXR {
	var xr PreviewXR
	xr.Metadata.Name = "incident-tracker-pr-42"
	xr.Spec.Crossplane.CompositionRef.Name = "preview-" + mode
	xr.Spec.ServiceRef = "incident-tracker"
	xr.Spec.PR.Number = 42
	xr.Spec.PR.HeadSHA = strings.Repeat("a", 40)
	xr.Spec.Image.Digest = "ghcr.io/demo/incident-tracker@sha256:" + strings.Repeat("b", 64)
	xr.Spec.Request.Size = "small"
	xr.Spec.Request.TTLMinutes = 60
	xr.Spec.Preview.Host = "incident-tracker-pr-42.localhost"
	xr.Spec.Decision.Mode = mode
	return xr
}

func TestAllowedClusterCapabilityMatchesRenderedCRD(t *testing.T) {
	data, err := os.ReadFile("../../backstage/templates/request-preview/cluster/deploy/cluster/incident-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var requested map[string]any
	if err := json.Unmarshal(data, &requested); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requested, incidentPolicyCRD()) {
		t.Fatal("Backstage-requested CRD differs from the fixed CRD rendered inside vCluster")
	}
}

func TestRenderNamespace(t *testing.T) {
	resources, err := Render(sampleXR("namespace"), "namespace")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 5 {
		t.Fatalf("got %d resources", len(resources))
	}
	if resources[2].Object["kind"] != "Deployment" {
		t.Fatalf("missing Deployment: %#v", resources[2])
	}
	if resources[4].Object["kind"] != "Ingress" {
		t.Fatalf("missing Ingress: %#v", resources[4])
	}
}

func TestRenderVCluster(t *testing.T) {
	xr := sampleXR("vcluster")
	xr.Spec.Capabilities = []string{"incident-policy"}
	resources, err := Render(xr, "vcluster")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 || resources[1].Object["kind"] != "Release" {
		t.Fatalf("missing vCluster Release: %#v", resources)
	}
	spec := resources[1].Object["spec"].(map[string]any)
	values := spec["forProvider"].(map[string]any)["values"].(map[string]any)
	image := values["controlPlane"].(map[string]any)["statefulSet"].(map[string]any)["image"].(map[string]any)
	if image["repository"] != "loft-sh/vcluster-oss" {
		t.Fatalf("vCluster must use its OSS image: %#v", image)
	}
	manifests := values["experimental"].(map[string]any)["deploy"].(map[string]any)["vcluster"].(map[string]any)["manifests"].(string)
	if !strings.Contains(manifests, "CustomResourceDefinition") || !strings.Contains(manifests, xr.Spec.Image.Digest) {
		t.Fatalf("virtual cluster does not receive CRD and app: %s", manifests)
	}
}

func withDeploymentSettings(xr PreviewXR) PreviewXR {
	xr.Spec.Deployment = &WorkloadDeployment{Replicas: 2}
	xr.Spec.Deployment.Resources.Requests = ResourceAmount{CPU: "250m", Memory: "256Mi"}
	xr.Spec.Deployment.Resources.Limits = ResourceAmount{CPU: "1000m", Memory: "1024Mi"}
	return xr
}

func TestRenderNamespaceDeploymentSettings(t *testing.T) {
	xr := withDeploymentSettings(sampleXR("namespace"))
	resources, err := Render(xr, "namespace")
	if err != nil {
		t.Fatal(err)
	}
	quota := resources[1].Object["spec"].(map[string]any)["hard"].(map[string]any)
	if quota["requests.cpu"] != "750m" || quota["requests.memory"] != "768Mi" {
		t.Fatalf("namespace quota has no surge capacity: %#v", quota)
	}
	deployment := resources[2].Object
	spec := deployment["spec"].(map[string]any)
	container := spec["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
	settings := container["resources"].(map[string]any)
	if spec["replicas"] != 2 || settings["requests"].(map[string]any)["cpu"] != "250m" ||
		settings["limits"].(map[string]any)["memory"] != "1024Mi" {
		t.Fatalf("namespace deployment did not receive settings: %#v", deployment)
	}
}

func TestRenderVClusterDeploymentSettings(t *testing.T) {
	xr := withDeploymentSettings(sampleXR("vcluster"))
	xr.Spec.Capabilities = []string{"incident-policy"}
	resources, err := Render(xr, "vcluster")
	if err != nil {
		t.Fatal(err)
	}
	values := resources[1].Object["spec"].(map[string]any)["forProvider"].(map[string]any)["values"].(map[string]any)
	manifests := values["experimental"].(map[string]any)["deploy"].(map[string]any)["vcluster"].(map[string]any)["manifests"].(string)
	var deployment map[string]any
	for _, part := range strings.Split(manifests, "\n---\n") {
		var object map[string]any
		if err := json.Unmarshal([]byte(part), &object); err != nil {
			t.Fatal(err)
		}
		if object["kind"] == "Deployment" {
			deployment = object
		}
	}
	if deployment == nil {
		t.Fatal("virtual cluster manifests have no Deployment")
	}
	spec := deployment["spec"].(map[string]any)
	container := spec["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
	settings := container["resources"].(map[string]any)
	if spec["replicas"] != float64(2) || settings["requests"].(map[string]any)["cpu"] != "250m" ||
		settings["limits"].(map[string]any)["memory"] != "1024Mi" {
		t.Fatalf("virtual cluster did not receive deployment config: %#v", deployment)
	}
}

func TestRenderRejectsMismatch(t *testing.T) {
	xr := sampleXR("namespace")
	if _, err := Render(xr, "vcluster"); err == nil {
		t.Fatal("expected mode mismatch")
	}
	xr.Spec.Image.Digest = "ghcr.io/demo/incident-tracker:latest"
	if _, err := Render(xr, "namespace"); err == nil {
		t.Fatal("expected immutable image check")
	}
	xr = sampleXR("namespace")
	xr.Spec.Deployment = &WorkloadDeployment{Replicas: 2}
	if _, err := Render(xr, "namespace"); err == nil {
		t.Fatal("expected invalid namespace deployment settings rejection")
	}
	xr = sampleXR("vcluster")
	xr.Spec.Deployment = &WorkloadDeployment{Replicas: 3}
	if _, err := Render(xr, "vcluster"); err == nil {
		t.Fatal("expected out-of-bounds deployment rejection")
	}
}
