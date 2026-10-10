package preview

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const CandidateWorkflow = "Preview Crossplane candidate"

type CandidateRecord struct {
	HeadSHA       string `json:"headSHA"`
	PackageDigest string `json:"packageDigest"`
	PackageTag    string `json:"packageTag"`
	WorkflowRunID int64  `json:"workflowRunID"`
	Branch        string `json:"-"`
}

type candidatePull struct {
	State string `json:"state"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		SHA  string `json:"sha"`
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
}

func (reader GitHubReader) candidatePull(ctx context.Context, repository string, number int) (candidatePull, string, error) {
	var pr candidatePull
	repo, err := repoPath(repository)
	if err != nil || number < 1 {
		return pr, "", errors.New("invalid repository or PR number")
	}
	if err := reader.getJSON(ctx, repo+"/pulls/"+strconv.Itoa(number), &pr); err != nil {
		return pr, "", err
	}
	if !shaPattern.MatchString(pr.Head.SHA) || pr.Head.Ref == "" {
		return pr, "", errors.New("PR has no valid source head")
	}
	return pr, repo, nil
}

// CandidateForPR accepts only a successful push run and its artifact for the
// current head of an open, trusted, same-repository PR. The digest is never read
// from PR source files.
func (reader GitHubReader) CandidateForPR(ctx context.Context, config Config, number int) (CandidateRecord, error) {
	var record CandidateRecord
	pr, repo, err := reader.candidatePull(ctx, config.Repository, number)
	if err != nil {
		return record, err
	}
	if pr.State != "open" || pr.Head.Repo.FullName != config.Repository || !contains(config.TrustedAuthors, pr.User.Login) {
		return record, errors.New("candidate PR is closed, forked, or untrusted")
	}
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
	path := repo + "/actions/runs?head_sha=" + url.QueryEscape(pr.Head.SHA) + "&per_page=100"
	if err := reader.getJSON(ctx, path, &runs); err != nil {
		return record, err
	}
	var runID int64
	var runStatus, conclusion string
	for _, run := range runs.WorkflowRuns {
		if run.Name == CandidateWorkflow && run.Event == "push" && run.HeadSHA == pr.Head.SHA && run.ID > runID {
			runID, runStatus, conclusion = run.ID, run.Status, run.Conclusion
		}
	}
	if runID == 0 {
		return record, errors.New("no candidate build for the current PR head")
	}
	if runStatus != "completed" || conclusion != "success" {
		return record, fmt.Errorf("current-head candidate build is %s/%s", runStatus, conclusion)
	}
	var artifacts struct {
		Artifacts []struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			Expired bool   `json:"expired"`
		} `json:"artifacts"`
	}
	if err := reader.getJSON(ctx, fmt.Sprintf("%s/actions/runs/%d/artifacts", repo, runID), &artifacts); err != nil {
		return record, err
	}
	var artifactID int64
	for _, artifact := range artifacts.Artifacts {
		if artifact.Name == "crossplane-candidate-ci-"+pr.Head.SHA && !artifact.Expired && artifact.ID > artifactID {
			artifactID = artifact.ID
		}
	}
	if artifactID == 0 {
		return record, errors.New("candidate artifact for current head is missing or expired")
	}
	data, err := reader.get(ctx, fmt.Sprintf("%s/actions/artifacts/%d/zip", repo, artifactID), 2<<20)
	if err != nil {
		return record, err
	}
	if err := artifactJSON(data, "crossplane-candidate-ci.json", &record); err != nil {
		return record, err
	}
	owner := strings.ToLower(strings.Split(config.Repository, "/")[0])
	packagePrefix := "ghcr.io/" + owner + "/function-preview-resources"
	tagPrefix := packagePrefix + ":v0.0.0-sha-" + pr.Head.SHA + "-run-" + strconv.FormatInt(runID, 10) + "-attempt-"
	if record.HeadSHA != pr.Head.SHA || record.WorkflowRunID != runID ||
		!regexp.MustCompile(`^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$`).MatchString(record.PackageDigest) ||
		!strings.HasPrefix(record.PackageDigest, packagePrefix+"@sha256:") ||
		!strings.HasPrefix(record.PackageTag, tagPrefix) ||
		!regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(strings.TrimPrefix(record.PackageTag, tagPrefix)) {
		return CandidateRecord{}, errors.New("candidate artifact does not match the PR head, run, or package repository")
	}
	record.Branch = pr.Head.Ref
	return record, nil
}
