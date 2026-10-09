package preview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var servicePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,35}$`)
var previewPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func PreviewName(service string, number int) (string, error) {
	if !servicePattern.MatchString(service) || number < 1 {
		return "", errors.New("invalid service or PR number")
	}
	name := fmt.Sprintf("%s-pr-%d", service, number)
	if len(name) > 63 {
		return "", errors.New("preview name is too long")
	}
	return name, nil
}

func MakeXR(snapshot Snapshot, result Decision, config Config) (map[string]any, error) {
	if result.Phase != "approved" || (result.Mode != "namespace" && result.Mode != "vcluster") {
		return nil, errors.New("only approved decisions can produce an XR")
	}
	name, err := PreviewName(config.Service, snapshot.PR.Number)
	if err != nil {
		return nil, err
	}
	spec := map[string]any{
		"crossplane":   map[string]any{"compositionRef": map[string]string{"name": "preview-" + result.Mode}},
		"serviceRef":   config.Service,
		"pr":           map[string]any{"number": snapshot.PR.Number, "headSHA": snapshot.PR.HeadSHA},
		"image":        map[string]string{"digest": result.ImageDigest},
		"request":      result.Request,
		"preview":      map[string]string{"host": name + ".localhost"},
		"decision":     map[string]any{"mode": result.Mode, "reasonCodes": result.ReasonCodes},
		"capabilities": result.Capabilities,
	}
	if result.Deployment != nil {
		spec["deployment"] = result.Deployment
	}
	if result.IncidentPolicy != nil {
		spec["incidentPolicy"] = result.IncidentPolicy
	}
	return map[string]any{
		"apiVersion": "preview.platform.example.org/v1alpha1", "kind": "PreviewEnvironment",
		"metadata": map[string]any{
			"name": name,
			"labels": map[string]string{
				"preview.platform.example.org/service": config.Service,
				"preview.platform.example.org/pr":      fmt.Sprint(snapshot.PR.Number),
				"app.kubernetes.io/managed-by":         "preview-evaluator",
			},
		},
		"spec": spec,
	}, nil
}

type Store struct {
	Root        string
	Now         func() time.Time
	PreviewPort int
	Readiness   ReadinessChecker
}

type statusRecord struct {
	Name             string      `json:"name"`
	PR               PullRequest `json:"pr"`
	Decision         Decision    `json:"decision"`
	UpdatedAt        string      `json:"updatedAt"`
	FirstApprovedAt  string      `json:"firstApprovedAt,omitempty"`
	ExpiresAt        string      `json:"expiresAt,omitempty"`
	CleanupStartedAt string      `json:"cleanupStartedAt,omitempty"`
}

func (store Store) readStatus(name string) (statusRecord, error) {
	if _, err := store.previewDir(name); err != nil {
		return statusRecord{}, err
	}
	data, err := os.ReadFile(filepath.Join(store.Root, "status", name+".json"))
	if err != nil {
		return statusRecord{}, err
	}
	var record statusRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return statusRecord{}, err
	}
	if record.Name != name {
		return statusRecord{}, errors.New("status record name mismatch")
	}
	return record, nil
}

func (store Store) now() time.Time {
	if store.Now != nil {
		return store.Now().UTC()
	}
	return time.Now().UTC()
}

// ActiveCount excludes the PR being updated, so a full pool can still update
// an existing preview without consuming a second capacity slot.
func (store Store) ActiveCount(except string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(store.Root, "previews"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != except {
			count++
		}
	}
	return count, nil
}

func (store Store) previewDir(name string) (string, error) {
	if !previewPattern.MatchString(name) || len(name) > 63 {
		return "", errors.New("invalid preview name")
	}
	return filepath.Join(store.Root, "previews", name), nil
}

func atomicJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".preview-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

func (store Store) WriteXR(xr map[string]any) (string, error) {
	metadata, ok := xr["metadata"].(map[string]any)
	if !ok {
		return "", errors.New("XR metadata missing")
	}
	name, ok := metadata["name"].(string)
	if !ok {
		return "", errors.New("XR name missing")
	}
	dir, err := store.previewDir(name)
	if err != nil {
		return "", err
	}
	filename := filepath.Join(dir, "previewenvironment.json")
	return filename, atomicJSON(filename, xr)
}

func (store Store) RemoveXR(name string) (bool, error) {
	dir, err := store.previewDir(name)
	if err != nil {
		return false, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() ||
			(entry.Name() != "previewenvironment.json" && entry.Name() != installerRoleFile && entry.Name() != installerBindingFile) {
			return false, errors.New("preview directory contains unexpected file")
		}
	}
	return true, os.RemoveAll(dir)
}

func (store Store) WriteStatus(name string, snapshot Snapshot, result Decision, firstApprovedAt, expiresAt string) error {
	if _, err := store.previewDir(name); err != nil {
		return err
	}
	filename := filepath.Join(store.Root, "status", name+".json")
	saved, err := store.readStatus(name)
	if err == nil {
		oldPR, _ := json.Marshal(saved.PR)
		newPR, _ := json.Marshal(snapshot.PR)
		oldDecision, _ := json.Marshal(saved.Decision)
		newDecision, _ := json.Marshal(result)
		if bytes.Equal(oldPR, newPR) && bytes.Equal(oldDecision, newDecision) && saved.FirstApprovedAt == firstApprovedAt && saved.ExpiresAt == expiresAt {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	status := statusRecord{Name: name, PR: snapshot.PR, Decision: result,
		UpdatedAt: store.now().Format(time.RFC3339Nano), FirstApprovedAt: firstApprovedAt, ExpiresAt: expiresAt}
	if result.Phase == "cleaning" || result.Phase == "cleanup-failed" {
		status.CleanupStartedAt = saved.CleanupStartedAt
		if status.CleanupStartedAt == "" {
			status.CleanupStartedAt = status.UpdatedAt
		}
	}
	return atomicJSON(filename, status)
}

func (store Store) Reconcile(snapshot Snapshot, config Config) (Decision, error) {
	name, err := PreviewName(config.Service, snapshot.PR.Number)
	if err != nil {
		return Decision{}, err
	}
	previous, readErr := store.readStatus(name)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return Decision{}, fmt.Errorf("read previous status: %w", readErr)
	}
	firstApprovedAt, expiresAt := previous.FirstApprovedAt, previous.ExpiresAt
	var result Decision
	if snapshot.PR.State == "closed" || snapshot.PR.State == "merged" {
		reason := "pr-closed"
		if snapshot.PR.State == "merged" {
			reason = "pr-merged"
		}
		result = decision(snapshot, "cleaning", "", reason)
		result.Mode = previous.Decision.Mode
		if previous.PR == snapshot.PR && (previous.Decision.Phase == "cleaning" || previous.Decision.Phase == "deleted" || previous.Decision.Phase == "cleanup-failed") {
			result = previous.Decision
		}
		_, err = store.RemoveXR(name)
	} else {
		previouslyExpired := false
		if firstApprovedAt != "" && expiresAt != "" {
			deadline, parseErr := time.Parse(time.RFC3339Nano, expiresAt)
			if parseErr != nil {
				return Decision{}, fmt.Errorf("invalid saved expiry: %w", parseErr)
			}
			previouslyExpired = !store.now().Before(deadline)
		}
		if previouslyExpired {
			result = decision(snapshot, "cleaning", previous.Decision.Mode, "ttl-expired", expiresAt)
			if previous.PR == snapshot.PR && (previous.Decision.Phase == "cleaning" || previous.Decision.Phase == "deleted" || previous.Decision.Phase == "cleanup-failed") && len(previous.Decision.ReasonCodes) > 0 && previous.Decision.ReasonCodes[0] == "ttl-expired" {
				result = previous.Decision
			}
			_, err = store.RemoveXR(name)
		} else {
			result = Evaluate(snapshot, config)
		}
		if !previouslyExpired && result.Phase == "approved" {
			if firstApprovedAt == "" {
				firstApprovedAt = store.now().Format(time.RFC3339Nano)
			}
			firstApproved, parseErr := time.Parse(time.RFC3339Nano, firstApprovedAt)
			if parseErr != nil {
				return Decision{}, fmt.Errorf("invalid saved first approval: %w", parseErr)
			}
			// A request update changes the lifetime without resetting its start.
			expires := firstApproved.Add(time.Duration(snapshot.Request.TTLMinutes) * time.Minute)
			expiresAt = expires.Format(time.RFC3339Nano)
			if !store.now().Before(expires) {
				result = decision(snapshot, "cleaning", result.Mode, "ttl-expired", expiresAt)
				_, err = store.RemoveXR(name)
			} else {
				if result.Mode == "vcluster" {
					err = store.WriteInstallerGrant(name, snapshot, result, config)
				} else {
					err = store.RemoveInstallerGrant(name)
				}
				var xr map[string]any
				if err == nil {
					xr, err = MakeXR(snapshot, result, config)
				}
				if err == nil {
					_, err = store.WriteXR(xr)
				}
			}
		} else if !previouslyExpired {
			_, err = store.RemoveXR(name) // Prevent serving an old commit while the new head awaits CI.
		}
	}
	if err != nil {
		return result, err
	}
	return result, store.WriteStatus(name, snapshot, result, firstApprovedAt, expiresAt)
}
