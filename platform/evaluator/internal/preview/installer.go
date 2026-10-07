package preview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	installerRoleFile    = "helm-installer-role.json"
	installerBindingFile = "helm-installer-binding.json"
)

// InstallerGrant is authored only for an evaluator-approved vCluster preview.
// Argo CD applies these fixed namespaced resources from the trusted GitOps
// checkout; Crossplane does not receive Role or RoleBinding write permission.
func InstallerGrant(name string, snapshot Snapshot, config Config) (map[string]any, map[string]any) {
	metadata := func() map[string]any {
		return map[string]any{
			"name": "preview-helm-installer", "namespace": name,
			"labels": map[string]string{
				"preview.platform.example.org/service": config.Service,
				"preview.platform.example.org/pr":      fmt.Sprint(snapshot.PR.Number),
				"app.kubernetes.io/managed-by":         "preview-evaluator",
			},
			// The namespace is owned by Crossplane. Keep the grant until
			// provider-helm uninstalls the Release, then namespace deletion
			// removes the Role and RoleBinding together.
			"annotations": map[string]string{"argocd.argoproj.io/sync-options": "Delete=false"},
		}
	}
	rule := func(groups, resources, verbs []string) map[string]any {
		return map[string]any{"apiGroups": groups, "resources": resources, "verbs": verbs}
	}
	verbs := []string{"get", "list", "watch", "create", "update", "patch", "delete"}
	role := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": metadata(),
		"rules": []any{
			rule([]string{""}, []string{"configmaps", "secrets", "services", "serviceaccounts", "pods", "pods/attach", "pods/portforward", "pods/exec", "pods/status", "pods/ephemeralcontainers", "pods/resize", "pods/log", "persistentvolumeclaims", "endpoints", "events"}, verbs),
			rule([]string{"apps"}, []string{"statefulsets", "replicasets", "deployments"}, verbs),
			rule([]string{"networking.k8s.io"}, []string{"ingresses"}, verbs),
			rule([]string{"discovery.k8s.io"}, []string{"endpointslices"}, verbs),
			rule([]string{"events.k8s.io"}, []string{"events"}, verbs),
			rule([]string{"rbac.authorization.k8s.io"}, []string{"roles", "rolebindings"}, verbs),
		},
	}
	binding := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": metadata(),
		"roleRef":  map[string]string{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "preview-helm-installer"},
		"subjects": []any{map[string]string{"kind": "ServiceAccount", "name": "preview-provider-helm", "namespace": "crossplane-system"}},
	}
	return role, binding
}

func (store Store) WriteInstallerGrant(name string, snapshot Snapshot, result Decision, config Config) error {
	expected, err := PreviewName(config.Service, snapshot.PR.Number)
	if err != nil {
		return err
	}
	if name != expected || result.Phase != "approved" || result.Mode != "vcluster" || snapshot.PR.State != "open" {
		return errors.New("installer grant requires an approved vCluster preview")
	}
	dir, err := store.previewDir(name)
	if err != nil {
		return err
	}
	role, binding := InstallerGrant(name, snapshot, config)
	if err := atomicJSON(filepath.Join(dir, installerRoleFile), role); err != nil {
		return err
	}
	return atomicJSON(filepath.Join(dir, installerBindingFile), binding)
}

func (store Store) RemoveInstallerGrant(name string) error {
	dir, err := store.previewDir(name)
	if err != nil {
		return err
	}
	for _, filename := range []string{installerRoleFile, installerBindingFile} {
		if err := os.Remove(filepath.Join(dir, filename)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
