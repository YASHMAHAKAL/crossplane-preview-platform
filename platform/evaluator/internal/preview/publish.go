package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Publish commits only evaluator-owned paths in a dedicated trusted checkout.
// A non-fast-forward push fails; the operator must update the checkout and retry.
func (store Store) Publish(ctx context.Context, message string) error {
	run := func(args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", store.Root}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
		return output, nil
	}
	if _, err := run("rev-parse", "--is-inside-work-tree"); err != nil {
		return err
	}
	for _, area := range []string{"previews", "status"} {
		if err := os.MkdirAll(filepath.Join(store.Root, area), 0o755); err != nil {
			return err
		}
	}
	if err := store.validatePublishTree(); err != nil {
		return err
	}
	for _, area := range []string{"previews", "status"} {
		marker := filepath.Join(store.Root, area, ".gitkeep")
		if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(marker, nil, 0o644); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	check := exec.CommandContext(ctx, "git", "-C", store.Root, "diff", "--cached", "--quiet")
	if err := check.Run(); err != nil {
		return errors.New("trusted GitOps checkout has staged changes")
	}
	if _, err := run("add", "-A", "--", "previews", "status"); err != nil {
		return err
	}
	check = exec.CommandContext(ctx, "git", "-C", store.Root, "diff", "--cached", "--quiet")
	if err := check.Run(); err != nil {
		if _, err := run("commit", "-m", message); err != nil {
			return err
		}
	}
	_, err := run("push", "origin", "HEAD:main")
	return err
}

func (store Store) validatePublishTree() error {
	for _, area := range []string{"previews", "status"} {
		root := filepath.Join(store.Root, area)
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if relative == "." {
				if !entry.IsDir() {
					return fmt.Errorf("trusted GitOps %s root is not a directory", area)
				}
				return nil
			}
			if area == "previews" {
				if entry.IsDir() && filepath.Dir(relative) == "." && previewPattern.MatchString(entry.Name()) {
					return nil
				}
				if !entry.IsDir() && entry.Type().IsRegular() &&
					(relative == ".gitkeep" || (filepath.Dir(relative) != "." &&
						(entry.Name() == "previewenvironment.json" || entry.Name() == installerRoleFile || entry.Name() == installerBindingFile))) {
					return nil
				}
			} else if !entry.IsDir() && entry.Type().IsRegular() &&
				(relative == ".gitkeep" || (filepath.Dir(relative) == "." && strings.HasSuffix(relative, ".json") && previewPattern.MatchString(strings.TrimSuffix(relative, ".json")))) {
				return nil
			}
			return fmt.Errorf("unexpected path in trusted GitOps %s tree: %s", area, relative)
		}); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if area == "previews" {
			entries, err := os.ReadDir(root)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			for _, entry := range entries {
				if entry.IsDir() {
					if err := validateInstallerGrant(filepath.Join(root, entry.Name()), entry.Name()); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validateInstallerGrant(dir, name string) error {
	xrBytes, err := os.ReadFile(filepath.Join(dir, "previewenvironment.json"))
	if err != nil {
		return fmt.Errorf("preview %s has no XR: %w", name, err)
	}
	var xr struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			ServiceRef string `json:"serviceRef"`
			PR         struct {
				Number int `json:"number"`
			} `json:"pr"`
			Decision struct {
				Mode string `json:"mode"`
			} `json:"decision"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(xrBytes, &xr); err != nil {
		return fmt.Errorf("preview %s XR JSON: %w", name, err)
	}
	expectedName, err := PreviewName(xr.Spec.ServiceRef, xr.Spec.PR.Number)
	if err != nil || xr.Metadata.Name != name || expectedName != name {
		return fmt.Errorf("preview %s XR identity mismatch", name)
	}
	if xr.Spec.Decision.Mode != "vcluster" {
		for _, filename := range []string{installerRoleFile, installerBindingFile} {
			if _, err := os.Stat(filepath.Join(dir, filename)); err == nil {
				return fmt.Errorf("preview %s has installer grant without vCluster decision", name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	role, binding := InstallerGrant(name, Snapshot{PR: PullRequest{Number: xr.Spec.PR.Number}}, Config{Service: xr.Spec.ServiceRef})
	for filename, expected := range map[string]any{installerRoleFile: role, installerBindingFile: binding} {
		actualBytes, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			return fmt.Errorf("preview %s missing %s: %w", name, filename, err)
		}
		var actual any
		if err := json.Unmarshal(actualBytes, &actual); err != nil {
			return err
		}
		actualCanonical, _ := json.Marshal(actual)
		expectedCanonical, _ := json.Marshal(expected)
		if !bytes.Equal(actualCanonical, expectedCanonical) {
			return fmt.Errorf("preview %s has modified %s", name, filename)
		}
	}
	return nil
}
