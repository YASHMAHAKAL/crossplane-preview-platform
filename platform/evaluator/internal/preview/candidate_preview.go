package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CandidatePreviewState struct {
	HeadSHA       string `json:"headSHA"`
	PackageDigest string `json:"packageDigest"`
	HostUID       string `json:"hostUID"`
	State         string `json:"state"`
	Detail        string `json:"detail,omitempty"`
}

func (store Store) candidateStatePath(name string) (string, error) {
	if _, err := store.previewDir(name); err != nil {
		return "", err
	}
	return filepath.Join(store.Root, "status", name+"-c.json"), nil
}

func (store Store) CandidateState(name string) (CandidatePreviewState, error) {
	path, err := store.candidateStatePath(name)
	if err != nil {
		return CandidatePreviewState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CandidatePreviewState{}, err
	}
	var state CandidatePreviewState
	if err := json.Unmarshal(data, &state); err != nil {
		return CandidatePreviewState{}, err
	}
	return state, nil
}

func (store Store) WriteCandidateState(name string, state CandidatePreviewState) error {
	path, err := store.candidateStatePath(name)
	if err != nil {
		return err
	}
	if !shaPattern.MatchString(state.HeadSHA) || !digestPattern.MatchString(state.PackageDigest) ||
		(state.State != "ready" && state.State != "failed") {
		return errors.New("invalid candidate preview state")
	}
	if len(state.Detail) > 512 {
		state.Detail = state.Detail[:512]
	}
	return atomicJSON(path, state)
}

func (store Store) RemoveCandidateState(name string) error {
	path, err := store.candidateStatePath(name)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type ShellCandidateInstaller struct {
	ScriptPath  string
	KubeContext string
	Reader      GitHubReader
}

// Ensure installs the candidate into the GitOps-created vCluster. The shell
// script receives only a trusted child XR and an immutable package reference.
func (installer ShellCandidateInstaller) Ensure(ctx context.Context, store Store, config Config, snapshot Snapshot, result Decision) error {
	if result.Candidate == nil || result.Candidate.State != "success" || result.Candidate.HeadSHA != snapshot.PR.HeadSHA {
		return errors.New("candidate installer requires a current successful candidate decision")
	}
	name, err := PreviewName(config.Service, snapshot.PR.Number)
	if err != nil {
		return err
	}
	child, _, err := MakeCandidateXR(snapshot, result, config)
	if err != nil {
		return err
	}
	uidCommand := exec.CommandContext(ctx, "kubectl", "--context", installer.KubeContext, "-n", name,
		"get", "secret", "vc-"+name, "-o", "jsonpath={.metadata.uid}")
	uidBytes, err := uidCommand.Output()
	if err != nil || len(uidBytes) == 0 {
		return ErrCandidatePending
	}
	uid := string(uidBytes)
	old, err := store.CandidateState(name)
	if err == nil && old.State == "ready" && old.HeadSHA == snapshot.PR.HeadSHA &&
		old.PackageDigest == result.Candidate.PackageDigest && old.HostUID == uid {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp("", "preview-candidate-xr-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err := json.NewEncoder(temporary).Encode(child); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, installer.ScriptPath, name, result.Candidate.PackageDigest, temporary.Name(), snapshot.PR.HeadSHA)
	command.Env = append(os.Environ(), "PREVIEW_KUBE_CONTEXT="+installer.KubeContext)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 512 {
			detail = detail[len(detail)-512:]
		}
		_ = store.WriteCandidateState(name, CandidatePreviewState{HeadSHA: snapshot.PR.HeadSHA,
			PackageDigest: result.Candidate.PackageDigest, HostUID: uid, State: "failed", Detail: detail})
		return fmt.Errorf("candidate install: %w: %s", err, detail)
	}
	current, err := installer.Reader.CandidateForPR(ctx, config, snapshot.PR.Number)
	if err != nil || current.HeadSHA != snapshot.PR.HeadSHA || current.PackageDigest != result.Candidate.PackageDigest ||
		current.WorkflowRunID != result.Candidate.WorkflowRunID {
		return errors.New("candidate PR head or package changed during virtual installation")
	}
	return store.WriteCandidateState(name, CandidatePreviewState{HeadSHA: snapshot.PR.HeadSHA,
		PackageDigest: result.Candidate.PackageDigest, HostUID: uid, State: "ready"})
}
