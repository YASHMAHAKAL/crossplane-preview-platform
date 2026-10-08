package preview

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// GitHubReader uses only PR metadata, changed paths, and a successful push
// workflow artifact from the exact PR head. It never trusts a digest from PR code.
type GitHubReader struct {
	Client  *http.Client
	APIBase string
	Token   string
}

type githubHTTPError struct {
	Code int
	Path string
}

func (err githubHTTPError) Error() string {
	return fmt.Sprintf("GitHub %s: HTTP %d", err.Path, err.Code)
}

func (reader GitHubReader) get(ctx context.Context, path string, limit int64) ([]byte, error) {
	base := reader.APIBase
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if reader.Token != "" {
		req.Header.Set("Authorization", "Bearer "+reader.Token)
	}
	client := reader.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, githubHTTPError{Code: response.StatusCode, Path: path}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("GitHub response exceeds size limit")
	}
	return data, nil
}

func (reader GitHubReader) getJSON(ctx context.Context, path string, target any) error {
	data, err := reader.get(ctx, path, 2<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func repoPath(repository string) (string, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New("repository must be owner/name")
	}
	return "/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), nil
}

// PullNumbers includes closed PRs so the watcher can remove a preview after
// merge or closure. The demo bounds the scan to the latest 1,000 PRs.
type PullSummary struct {
	Number int    `json:"number"`
	State  string `json:"state"`
}

func (reader GitHubReader) PullNumbers(ctx context.Context, repository string) ([]PullSummary, error) {
	repo, err := repoPath(repository)
	if err != nil {
		return nil, err
	}
	var numbers []PullSummary
	for page := 1; page <= 10; page++ {
		var prs []PullSummary
		path := fmt.Sprintf("%s/pulls?state=all&sort=updated&direction=desc&per_page=100&page=%d", repo, page)
		if err := reader.getJSON(ctx, path, &prs); err != nil {
			return nil, err
		}
		numbers = append(numbers, prs...)
		if len(prs) < 100 {
			return numbers, nil
		}
	}
	return nil, errors.New("repository has more than 1000 PRs; narrow watcher scope")
}

func (reader GitHubReader) Snapshot(ctx context.Context, config Config, number, active int) (Snapshot, error) {
	var snapshot Snapshot
	repo, err := repoPath(config.Repository)
	if err != nil {
		return snapshot, err
	}
	var pr struct {
		Number   int     `json:"number"`
		State    string  `json:"state"`
		MergedAt *string `json:"merged_at"`
		User     struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	}
	if err := reader.getJSON(ctx, repo+"/pulls/"+strconv.Itoa(number), &pr); err != nil {
		return snapshot, err
	}
	state := pr.State
	if pr.MergedAt != nil {
		state = "merged"
	}
	snapshot.PR = PullRequest{Number: pr.Number, HeadSHA: pr.Head.SHA, Repository: pr.Head.Repo.FullName,
		Author: pr.User.Login, Fork: pr.Head.Repo.FullName != config.Repository, State: state}
	snapshot.ActivePreviews = active
	snapshot.Files = []ChangedFile{}
	snapshot.Request = config.PreviewDefaults()
	if state != "open" {
		return snapshot, nil
	}
	if snapshot.PR.Fork || !contains(config.TrustedAuthors, snapshot.PR.Author) {
		return snapshot, nil // Policy records the untrusted-pr decision.
	}
	for page := 1; page <= 10; page++ {
		var changed []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
		}
		path := fmt.Sprintf("%s/pulls/%d/files?per_page=100&page=%d", repo, number, page)
		if err := reader.getJSON(ctx, path, &changed); err != nil {
			return snapshot, err
		}
		for _, file := range changed {
			entry := ChangedFile{Path: file.Filename, Status: file.Status}
			if file.Status != "removed" && (entry.Path == DeploymentConfigPath ||
				(strings.HasPrefix(entry.Path, "deploy/cluster/") && strings.HasSuffix(entry.Path, ".json"))) {
				if strings.Contains(entry.Path, "..") || strings.Contains(entry.Path, "\\") {
					return snapshot, errors.New("invalid cluster manifest path")
				}
				content, err := reader.contentAtHead(ctx, repo, entry.Path, pr.Head.SHA)
				if err != nil {
					return snapshot, err
				}
				entry.Content = content
			}
			snapshot.Files = append(snapshot.Files, entry)
		}
		if len(changed) < 100 {
			break
		}
		if page == 10 {
			return snapshot, errors.New("PR has more than 1000 changed files")
		}
	}
	if err := reader.fillCI(ctx, repo, &snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (reader GitHubReader) contentAtHead(ctx context.Context, repo, filename, sha string) (string, error) {
	var content struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	path := repo + "/contents/" + filename + "?ref=" + url.QueryEscape(sha)
	if err := reader.getJSON(ctx, path, &content); err != nil {
		return "", err
	}
	if content.Encoding != "base64" {
		return "", errors.New("unsupported GitHub content encoding")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
	if err != nil {
		return "", err
	}
	if len(decoded) > 65536 {
		return "", errors.New("changed file is too large")
	}
	return string(decoded), nil
}

func (reader GitHubReader) fillCI(ctx context.Context, repo string, snapshot *Snapshot) error {
	var runs struct {
		WorkflowRuns []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Event      string `json:"event"`
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"workflow_runs"`
	}
	path := repo + "/actions/runs?head_sha=" + url.QueryEscape(snapshot.PR.HeadSHA) + "&per_page=100"
	if err := reader.getJSON(ctx, path, &runs); err != nil {
		return err
	}
	snapshot.CI = CI{State: "pending", HeadSHA: snapshot.PR.HeadSHA}
	for _, run := range runs.WorkflowRuns {
		if run.Name != "Preview image" || run.Event != "push" || run.HeadSHA != snapshot.PR.HeadSHA {
			continue
		}
		if run.Status != "completed" {
			return nil
		}
		if run.Conclusion != "success" {
			snapshot.CI.State = "failure"
			return nil
		}
		var artifacts struct {
			Artifacts []struct {
				Name    string `json:"name"`
				ID      int64  `json:"id"`
				Expired bool   `json:"expired"`
			} `json:"artifacts"`
		}
		if err := reader.getJSON(ctx, fmt.Sprintf("%s/actions/runs/%d/artifacts", repo, run.ID), &artifacts); err != nil {
			return err
		}
		for _, artifact := range artifacts.Artifacts {
			if artifact.Name != "preview-ci-"+snapshot.PR.HeadSHA || artifact.Expired {
				continue
			}
			data, err := reader.get(ctx, fmt.Sprintf("%s/actions/artifacts/%d/zip", repo, artifact.ID), 2<<20)
			if err != nil {
				return err
			}
			var record struct {
				HeadSHA     string `json:"headSHA"`
				ImageDigest string `json:"imageDigest"`
			}
			if err := artifactRecord(data, &record); err != nil {
				return err
			}
			if record.HeadSHA != snapshot.PR.HeadSHA {
				return errors.New("CI artifact head SHA does not match PR")
			}
			snapshot.CI = CI{State: "success", HeadSHA: record.HeadSHA, ImageDigest: record.ImageDigest}
			return nil
		}
		return nil // Successful run, artifact not available yet.
	}
	return nil
}

func artifactRecord(data []byte, target any) error {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range archive.File {
		if file.Name != "preview-ci.json" || file.UncompressedSize64 > 65536 {
			continue
		}
		body, err := file.Open()
		if err != nil {
			return err
		}
		contents, err := io.ReadAll(io.LimitReader(body, 65537))
		body.Close()
		if err != nil {
			return err
		}
		if len(contents) > 65536 {
			return errors.New("CI artifact is too large")
		}
		return json.Unmarshal(contents, target)
	}
	return errors.New("CI artifact missing preview-ci.json")
}
