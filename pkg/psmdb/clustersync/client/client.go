package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/percona/percona-server-mongodb-operator/pkg/psmdb/clustersync"
	"github.com/pkg/errors"

	api "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
)

func ServiceURL(cr *api.PerconaServerMongoDBClusterSync) string {
	return fmt.Sprintf("http://%s.%s.svc:%d", clustersync.DeploymentName(cr), cr.Namespace, clustersync.HTTPPort)
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Subset of GET /status; tuning/observability fields (initialSync.*, finalization.*,
// eventsRead, eventsApplied, lastReplicatedOpTime) are not yet propagated to the CR.
type Status struct {
	State          string `json:"state"`
	Info           string `json:"info,omitempty"`
	Error          string `json:"error,omitempty"`
	LagTimeSeconds int64  `json:"lagTimeSeconds,omitempty"`
}

type StartOptions struct {
	IncludeNamespaces []string `json:"includeNamespaces,omitempty"`
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`
}

// APIError carries PCSM's {"ok":false,"error":"..."} payload.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("pcsm api: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("pcsm api: HTTP %d: %s", e.StatusCode, e.Message)
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var out Status
	if err := c.do(ctx, http.MethodGet, "/status", nil, &out); err != nil {
		return Status{}, err
	}
	return out, nil
}

func (c *Client) Start(ctx context.Context, opts StartOptions) error {
	return c.do(ctx, http.MethodPost, "/start", opts, nil)
}

func (c *Client) Pause(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/pause", nil, nil)
}

// fromFailure=true is the documented recovery path for /resume out of a failed state.
func (c *Client) Resume(ctx context.Context, fromFailure bool) error {
	var body any
	if fromFailure {
		body = map[string]bool{"fromFailure": true}
	}
	return c.do(ctx, http.MethodPost, "/resume", body, nil)
}

// /reset is not in the published API page; verify against the deployed PCSM binary.
func (c *Client) Reset(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/reset", nil, nil)
}

func (c *Client) Finalize(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/finalize", nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return errors.Wrapf(err, "marshal %s body", path)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return errors.Wrapf(err, "new request %s", path)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return errors.Wrapf(err, "call %s", path)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			// TODO: add log
		}
	}(resp.Body)

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "read %s response", path)
	}

	if resp.StatusCode != http.StatusOK {
		return &APIError{StatusCode: resp.StatusCode, Message: extractErrorMessage(raw)}
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return errors.Wrapf(err, "decode %s response", path)
		}
	}
	return nil
}

func extractErrorMessage(raw []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &e); err == nil && e.Error != "" {
		return e.Error
	}
	return string(raw)
}
