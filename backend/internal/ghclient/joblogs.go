package ghclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/go-github/v91/github"
)

const maxJobLogBytes = 16 << 20

var logDownloader = &http.Client{Timeout: time.Minute}

func JobLogs(ctx context.Context, c *github.Client, owner, repo string, jobID int64) (string, *github.Response, error) {
	job := fmt.Sprintf("%s/%s/%d", owner, repo, jobID)
	u, resp, err := c.Actions.GetWorkflowJobLogs(ctx, owner, repo, jobID, 0)
	if err != nil {
		return "", resp, fmt.Errorf("logs of job %s: %w", job, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", resp, fmt.Errorf("logs of job %s: %w", job, err)
	}
	res, err := logDownloader.Do(req)
	if err != nil {
		return "", resp, fmt.Errorf("download the logs of job %s: %w", job, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", resp, downloadError(res.StatusCode, "logs of job "+job)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxJobLogBytes))
	if err != nil {
		return "", resp, fmt.Errorf("read the logs of job %s: %w", job, err)
	}
	return string(body), resp, nil
}
