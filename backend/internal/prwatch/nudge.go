package prwatch

import (
	"context"
	"encoding/json"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func toItems(rows []store.Activity) []agent.Item {
	out := make([]agent.Item, 0, len(rows))
	for _, a := range rows {
		out = append(out, toItem(a))
	}
	return out
}

func (s *Service) keepJobs(ctx context.Context, w store.Watch, snap *snapshot.Snapshot) error {
	if len(snap.FailedJobs) == 0 {
		return nil
	}
	rows, err := s.store.UnnudgedOfKind(ctx, w.ID, store.ActivityCheckFailed)
	if err != nil {
		return err
	}
	written := false
	for _, a := range rows {
		it := toItem(a)
		j, ok := jobOf(snap.FailedJobs, it.Check, it.RunID)
		if !ok || (j.RunID == it.RunID && j.JobID == it.JobID) {
			continue
		}
		if err := s.store.SetActivityJob(ctx, a.ID, withJobFields(a.Payload, j), j.HTMLURL); err != nil {
			return err
		}
		written = true
	}
	if written {
		s.store.PublishActivity(w.Key())
	}
	return nil
}

func withJobFields(payload json.RawMessage, j snapshot.FailedJob) json.RawMessage {
	p := map[string]any{}
	_ = json.Unmarshal(payload, &p)
	return mustJSON(withJob(p, []snapshot.FailedJob{j}, j.JobName, j.RunID))
}

func jobOf(jobs []snapshot.FailedJob, name string, runID int64) (snapshot.FailedJob, bool) {
	ofRun := jobsOfRun(jobs, runID)
	if j, ok := jobNamed(ofRun, name); ok {
		return j, true
	}
	if j, ok := jobNamed(jobs, name); ok {
		return j, true
	}
	if len(ofRun) == 0 {
		return snapshot.FailedJob{}, false
	}
	return ofRun[0], true
}

func jobsOfRun(jobs []snapshot.FailedJob, runID int64) []snapshot.FailedJob {
	if runID == 0 {
		return nil
	}
	var out []snapshot.FailedJob
	for _, j := range jobs {
		if j.RunID == runID {
			out = append(out, j)
		}
	}
	return out
}

func jobNamed(jobs []snapshot.FailedJob, name string) (snapshot.FailedJob, bool) {
	for _, j := range jobs {
		if j.JobName == name {
			return j, true
		}
	}
	return snapshot.FailedJob{}, false
}

func toItem(a store.Activity) agent.Item {
	it := agent.Item{ID: a.ID, Kind: a.Kind, Actor: a.Actor, At: a.At, URL: a.URL}
	var p struct {
		ItemID       int64             `json:"item_id"`
		Body         string            `json:"body"`
		Path         string            `json:"path"`
		Line         int               `json:"line"`
		State        string            `json:"state"`
		Check        string            `json:"check"`
		Conclusion   checks.Conclusion `json:"conclusion"`
		RunID        int64             `json:"run_id"`
		JobID        int64             `json:"job_id"`
		JobName      string            `json:"job_name"`
		LogsEndpoint string            `json:"logs_endpoint"`
		Base         string            `json:"base"`
	}
	_ = json.Unmarshal(a.Payload, &p)
	it.ItemID, it.Body, it.Path, it.Line, it.State = p.ItemID, p.Body, p.Path, p.Line, p.State
	it.Check, it.Conclusion, it.RunID, it.Base = p.Check, p.Conclusion, p.RunID, p.Base
	it.JobID, it.JobName, it.LogsEndpoint = p.JobID, p.JobName, p.LogsEndpoint
	return it
}
