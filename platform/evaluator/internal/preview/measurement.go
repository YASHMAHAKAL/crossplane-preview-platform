package preview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// PhaseSample is one raw observation. DurationMS is omitted when the observer
// started after the target phase, because that would understate latency.
type PhaseSample struct {
	Name          string       `json:"name"`
	Target        string       `json:"target"`
	Mode          string       `json:"mode,omitempty"`
	HeadSHA       string       `json:"headSHA,omitempty"`
	Outcome       string       `json:"outcome"`
	ObserverStart string       `json:"observerStart"`
	BaselineAt    string       `json:"baselineAt,omitempty"`
	ObservedAt    string       `json:"observedAt,omitempty"`
	DurationMS    *int64       `json:"durationMs,omitempty"`
	LateStart     bool         `json:"lateStart,omitempty"`
	PollInterval  string       `json:"pollInterval"`
	Events        []PhaseEvent `json:"events"`
	ReasonCodes   []string     `json:"reasonCodes,omitempty"`
	Evidence      []string     `json:"evidence,omitempty"`
	Error         string       `json:"error,omitempty"`
}

type PhaseEvent struct {
	At    string `json:"at"`
	Phase string `json:"phase"`
}

// ObservePhase samples the same Store.Status result used by the status API.
// The baseline is the evaluator's first approval or cleanup start, and the
// target timestamp is the first poll that observed ready or deleted.
func (store Store) ObservePhase(ctx context.Context, name, target, expectedMode string, interval time.Duration, client *http.Client) (PhaseSample, error) {
	if _, err := store.previewDir(name); err != nil {
		return PhaseSample{}, err
	}
	if target != "ready" && target != "deleted" {
		return PhaseSample{}, errors.New("target must be ready or deleted")
	}
	if expectedMode != "" && expectedMode != "namespace" && expectedMode != "vcluster" {
		return PhaseSample{}, errors.New("invalid expected mode")
	}
	if interval < time.Second {
		return PhaseSample{}, errors.New("poll interval must be at least one second")
	}
	now := store.now
	sample := PhaseSample{Name: name, Target: target, Outcome: "timeout", ObserverStart: now().Format(time.RFC3339Nano), PollInterval: interval.String(), Events: []PhaseEvent{}}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	seenBeforeTarget := false
	for {
		phase := "pending-evaluation"
		status, err := store.Status(name, client)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		} else if err == nil {
			phase = status.Phase
			sample.Mode = status.Mode
			sample.HeadSHA = status.HeadSHA
			sample.ReasonCodes = status.ReasonCodes
			sample.Evidence = status.Evidence
		}
		at := now()
		if err != nil {
			sample.Error = err.Error()
			phase = "observation-error"
		} else {
			sample.Error = ""
		}
		if len(sample.Events) == 0 || sample.Events[len(sample.Events)-1].Phase != phase {
			sample.Events = append(sample.Events, PhaseEvent{At: at.Format(time.RFC3339Nano), Phase: phase})
		}
		if phase != target {
			seenBeforeTarget = true
		}
		if phase == target {
			sample.ObservedAt = at.Format(time.RFC3339Nano)
			if expectedMode != "" && status.Mode != expectedMode {
				sample.Outcome = "wrong-mode"
				sample.Error = fmt.Sprintf("expected %s, observed %s", expectedMode, status.Mode)
				return sample, errors.New(sample.Error)
			}
			record, readErr := store.readStatus(name)
			if readErr != nil {
				sample.Outcome = "observation-error"
				sample.Error = readErr.Error()
				return sample, readErr
			}
			baseline := record.FirstApprovedAt
			if target == "deleted" {
				baseline = record.CleanupStartedAt
			}
			start, parseErr := time.Parse(time.RFC3339Nano, baseline)
			if parseErr != nil || baseline == "" || at.Before(start) {
				sample.Outcome = "invalid-baseline"
				sample.Error = "missing or invalid evaluator baseline timestamp"
				return sample, errors.New(sample.Error)
			}
			sample.BaselineAt = baseline
			if !seenBeforeTarget {
				sample.Outcome = "late-start"
				sample.LateStart = true
				return sample, nil
			}
			duration := at.Sub(start).Milliseconds()
			sample.DurationMS = &duration
			sample.Outcome = "observed"
			return sample, nil
		}
		// cleanup-failed can recover after the blocked resource disappears, so
		// keep observing it until deleted or the requested timeout.
		if target == "ready" && (phase == "rejected" || phase == "deleted" || phase == "expired") {
			sample.Outcome = "terminal-phase"
			sample.Error = "target not reached; observed " + phase
			return sample, errors.New(sample.Error)
		}
		select {
		case <-ctx.Done():
			sample.Error = ctx.Err().Error()
			return sample, ctx.Err()
		case <-ticker.C:
		}
	}
}
