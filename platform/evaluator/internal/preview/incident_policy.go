package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

const IncidentPolicyPath = "deploy/cluster/incident-policy.json"

// ParseIncidentPolicy accepts only the trusted baseline CRD or that same CRD
// with "critical" appended to the severity enum. It never forwards PR JSON.
func ParseIncidentPolicy(content string, allowed AllowedCRD) (IncidentPolicySpec, error) {
	var manifest map[string]any
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		return IncidentPolicySpec{}, err
	}
	spec, ok := manifest["spec"].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("CRD spec missing")
	}
	versions, ok := spec["versions"].([]any)
	if !ok || len(versions) != 1 {
		return IncidentPolicySpec{}, errors.New("unsupported CRD versions")
	}
	version, ok := versions[0].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("invalid CRD version")
	}
	schema, ok := version["schema"].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("CRD schema missing")
	}
	root, ok := schema["openAPIV3Schema"].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("CRD OpenAPI schema missing")
	}
	properties, ok := root["properties"].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("CRD properties missing")
	}
	policy, ok := properties["spec"].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("IncidentPolicy spec missing")
	}
	fields, ok := policy["properties"].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("IncidentPolicy fields missing")
	}
	severity, ok := fields["severity"].(map[string]any)
	if !ok {
		return IncidentPolicySpec{}, errors.New("severity field missing")
	}
	values, ok := severity["enum"].([]any)
	if !ok || (len(values) != 2 && len(values) != 3) || values[0] != "low" || values[1] != "high" {
		return IncidentPolicySpec{}, errors.New("unsupported severity enum")
	}
	result := IncidentPolicySpec{}
	if len(values) == 3 {
		if values[2] != "critical" {
			return IncidentPolicySpec{}, errors.New("unsupported severity value")
		}
		result.AllowCritical = true
	}
	severity["enum"] = []any{"low", "high"}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return IncidentPolicySpec{}, err
	}
	hash := sha256.Sum256(canonical)
	if hex.EncodeToString(hash[:]) != allowed.ManifestSHA256 {
		return IncidentPolicySpec{}, errors.New("CRD differs from trusted baseline outside severity enum")
	}
	return result, nil
}
