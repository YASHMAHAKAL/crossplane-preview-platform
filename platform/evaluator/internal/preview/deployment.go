package preview

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const DeploymentConfigPath = "deploy/incident-tracker/preview.json"

// ParseDeploymentConfig accepts only the bounded fields the platform can
// render. The raw PR document is never published to trusted GitOps.
func ParseDeploymentConfig(content string) (DeploymentSpec, error) {
	var document struct {
		APIVersion string         `json:"apiVersion"`
		Kind       string         `json:"kind"`
		Spec       DeploymentSpec `json:"spec"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return DeploymentSpec{}, fmt.Errorf("decode deployment config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return DeploymentSpec{}, errors.New("deployment config has trailing data")
	}
	if document.APIVersion != "preview.platform.example.org/v1alpha1" || document.Kind != "IncidentTrackerDeployment" {
		return DeploymentSpec{}, errors.New("unsupported deployment config version or kind")
	}
	if err := ValidateDeploymentSpec(document.Spec); err != nil {
		return DeploymentSpec{}, err
	}
	return document.Spec, nil
}

func ValidateDeploymentSpec(spec DeploymentSpec) error {
	if spec.Replicas < 1 || spec.Replicas > 2 ||
		!contains([]string{"100m", "250m"}, spec.Resources.Requests.CPU) ||
		!contains([]string{"128Mi", "256Mi"}, spec.Resources.Requests.Memory) ||
		!contains([]string{"500m", "1000m"}, spec.Resources.Limits.CPU) ||
		!contains([]string{"512Mi", "1024Mi"}, spec.Resources.Limits.Memory) {
		return errors.New("deployment values exceed supported replicas or resource bounds")
	}
	return nil
}
