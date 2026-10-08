package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

const PolicyVersion = "3"

var (
	shaPattern    = regexp.MustCompile(`^[a-f0-9]{40}$`)
	hashPattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
	digestPattern = regexp.MustCompile(`^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$`)
)

func decision(snapshot Snapshot, phase, mode, code string, evidence ...string) Decision {
	return Decision{PolicyVersion: PolicyVersion, Phase: phase, Mode: mode,
		ReasonCodes: []string{code}, Evidence: append([]string{}, evidence...), HeadSHA: snapshot.PR.HeadSHA}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func Evaluate(snapshot Snapshot, config Config) Decision {
	if config.Service == "" || config.Repository == "" || config.MaxTTLMinutes < 5 || config.MaxActivePreviews < 1 ||
		config.AllowedCRD.Name == "" || config.AllowedCRD.Group == "" || config.AllowedCRD.Kind == "" ||
		!hashPattern.MatchString(config.AllowedCRD.ManifestSHA256) ||
		snapshot.PR.Number < 1 || !shaPattern.MatchString(snapshot.PR.HeadSHA) || snapshot.Files == nil ||
		snapshot.ActivePreviews < 0 {
		return decision(snapshot, "rejected", "", "invalid-input")
	}
	if snapshot.PR.State != "open" {
		return decision(snapshot, "rejected", "", "pr-not-open")
	}
	if snapshot.PR.Repository != config.Repository || snapshot.PR.Fork || !contains(config.TrustedAuthors, snapshot.PR.Author) {
		return decision(snapshot, "rejected", "", "untrusted-pr", snapshot.PR.Repository, snapshot.PR.Author)
	}
	if _, ok := config.Sizes[snapshot.Request.Size]; !ok || snapshot.Request.TTLMinutes < 5 ||
		snapshot.Request.TTLMinutes > config.MaxTTLMinutes {
		return decision(snapshot, "rejected", "", "invalid-request", "size or TTL outside published bounds")
	}
	needsVirtualCluster := false
	deploymentChanged := false
	var deployment DeploymentSpec
	evidence := []string{}
	appChanges := []string{}
	for _, file := range snapshot.Files {
		if file.Path == "" || strings.Contains(file.Path, "\\") || strings.HasPrefix(file.Path, "/") ||
			path.Clean(file.Path) != file.Path || strings.HasPrefix(file.Path, "../") {
			return decision(snapshot, "rejected", "", "invalid-file-list")
		}
		if file.Path == DeploymentConfigPath {
			parsed, err := ParseDeploymentConfig(file.Content)
			if err != nil || file.Status == "removed" {
				return decision(snapshot, "rejected", "", "invalid-deployment-config", file.Path)
			}
			deployment = parsed
			deploymentChanged = true
			evidence = append(evidence, fmt.Sprintf("%s:replicas=%d", file.Path, parsed.Replicas))
			continue
		}
		if strings.HasPrefix(file.Path, "deploy/cluster/") {
			if !strings.HasSuffix(file.Path, ".json") || file.Content == "" {
				return decision(snapshot, "rejected", "", "unsupported-manifest-format", file.Path)
			}
			var manifest struct {
				APIVersion string `json:"apiVersion"`
				Kind       string `json:"kind"`
				Metadata   struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Spec struct {
					Group string `json:"group"`
					Scope string `json:"scope"`
					Names struct {
						Kind string `json:"kind"`
					} `json:"names"`
				} `json:"spec"`
			}
			if err := json.Unmarshal([]byte(file.Content), &manifest); err != nil {
				return decision(snapshot, "rejected", "", "invalid-manifest", file.Path)
			}
			if manifest.APIVersion != "apiextensions.k8s.io/v1" || manifest.Kind != "CustomResourceDefinition" ||
				manifest.Metadata.Name != config.AllowedCRD.Name || manifest.Spec.Group != config.AllowedCRD.Group ||
				manifest.Spec.Names.Kind != config.AllowedCRD.Kind || manifest.Spec.Scope != "Namespaced" {
				return decision(snapshot, "rejected", "", "unsupported-cluster-resource", file.Path)
			}
			var canonical map[string]any
			if err := json.Unmarshal([]byte(file.Content), &canonical); err != nil {
				return decision(snapshot, "rejected", "", "invalid-manifest", file.Path)
			}
			encoded, _ := json.Marshal(canonical)
			hash := sha256.Sum256(encoded)
			if hex.EncodeToString(hash[:]) != config.AllowedCRD.ManifestSHA256 {
				return decision(snapshot, "rejected", "", "unsupported-cluster-resource", file.Path+":schema-mismatch")
			}
			needsVirtualCluster = true
			evidence = append(evidence, file.Path+":CustomResourceDefinition/"+manifest.Metadata.Name)
			continue
		}
		if strings.HasPrefix(file.Path, "app/incident-tracker/") {
			appChanges = append(appChanges, file.Path)
			continue
		}
		if file.Path == "preview.request.json" || file.Path == "README.md" || strings.HasPrefix(file.Path, "docs/") {
			continue // Legacy request files and documentation do not request a preview.
		}
		return decision(snapshot, "rejected", "", "unsupported-change", file.Path)
	}
	if !needsVirtualCluster && !deploymentChanged && len(appChanges) == 0 {
		return decision(snapshot, "skipped", "", "no-previewable-change")
	}
	if snapshot.ActivePreviews >= config.MaxActivePreviews {
		return decision(snapshot, "rejected", "", "capacity-exceeded", fmt.Sprintf("active=%d", snapshot.ActivePreviews))
	}
	if snapshot.CI.HeadSHA != snapshot.PR.HeadSHA || snapshot.CI.State == "pending" {
		return decision(snapshot, "waiting-for-ci", "", "ci-not-current")
	}
	if snapshot.CI.State != "success" {
		return decision(snapshot, "rejected", "", "ci-failed")
	}
	if !digestPattern.MatchString(snapshot.CI.ImageDigest) ||
		!strings.HasPrefix(snapshot.CI.ImageDigest, "ghcr.io/"+strings.ToLower(config.Repository)+"@sha256:") {
		return decision(snapshot, "rejected", "", "invalid-image-digest")
	}
	mode, reason := "namespace", "namespaced-app-change"
	capabilities := []string{}
	if needsVirtualCluster {
		mode, reason = "vcluster", "cluster-api-required"
		capabilities = append(capabilities, "incident-policy")
	} else if deploymentChanged {
		mode, reason = "vcluster", "deployment-stack-change"
	} else if len(appChanges) > 0 {
		evidence = append(evidence, appChanges[0])
	}
	result := decision(snapshot, "approved", mode, reason, evidence...)
	result.Capabilities = capabilities
	result.ImageDigest = snapshot.CI.ImageDigest
	result.Request = &snapshot.Request
	if deploymentChanged {
		result.Deployment = &deployment
	}
	return result
}
