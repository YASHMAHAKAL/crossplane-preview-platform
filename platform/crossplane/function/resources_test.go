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
	manifests := values["experimental"].(map[string]any)["deploy"].(map[string]any)["vcluster"].(map[string]any)["manifests"].(string)
	if !strings.Contains(manifests, "CustomResourceDefinition") || !strings.Contains(manifests, xr.Spec.Image.Digest) {
		t.Fatalf("virtual cluster does not receive CRD and app: %s", manifests)
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
}
