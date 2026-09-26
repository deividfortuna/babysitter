package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/runfile"
)

var errNoDaemon = errors.New("no daemon is running; start it with 'babysitter daemon start' or open the desktop app")

type daemonError struct {
	Status  int
	Code    string
	Message string
	Body    []byte
}

func (e *daemonError) Error() string { return e.Message }

func errorCode(err error) string {
	if de, ok := errors.AsType[*daemonError](err); ok {
		return de.Code
	}
	return ""
}

type daemonClient struct {
	base string
	http *http.Client
}

func (o *options) daemonClient(dataDirFlag string) (*daemonClient, error) {
	dataDir, err := o.dataDir(dataDirFlag)
	if err != nil {
		return nil, err
	}
	info, err := runfile.Live(runfile.Path(dataDir))
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errNoDaemon
	}
	return &daemonClient{
		base: fmt.Sprintf("http://127.0.0.1:%d%s", info.Port, httpd.Prefix),
		http: &http.Client{Timeout: o.timeout},
	}, nil
}

func (c *daemonClient) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *daemonClient) post(ctx context.Context, path string, in, out any) error {
	return c.do(ctx, http.MethodPost, path, in, out)
}

func (c *daemonClient) put(ctx context.Context, path string, in, out any) error {
	return c.do(ctx, http.MethodPut, path, in, out)
}

func (c *daemonClient) patch(ctx context.Context, path string, in, out any) error {
	return c.do(ctx, http.MethodPatch, path, in, out)
}

func (c *daemonClient) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call the daemon: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read the daemon response: %w", err)
	}
	if resp.StatusCode >= 400 {
		var apiErr httpd.APIError
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error.Message != "" {
			return &daemonError{Status: resp.StatusCode, Code: apiErr.Error.Code, Message: apiErr.Error.Message, Body: data}
		}
		return fmt.Errorf("the daemon answered %s", resp.Status)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode the daemon response: %w", err)
	}
	return nil
}
