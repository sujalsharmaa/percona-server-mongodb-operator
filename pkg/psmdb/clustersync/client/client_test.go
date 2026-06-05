package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Status(t *testing.T) {
	tests := map[string]struct {
		handler    http.HandlerFunc
		want       Status
		wantErrMsg string
		wantAPI    *APIError
	}{
		"replicating": {
			handler: jsonHandler(t, http.MethodGet, "/status", "", http.StatusOK, `{
				"ok": true,
				"state": "replicating",
				"lagTimeSeconds": 3,
				"info": "all good"
			}`),
			want: Status{State: "replicating", LagTimeSeconds: 3, Info: "all good"},
		},
		"failed surfaces error field but still 200": {
			handler: jsonHandler(t, http.MethodGet, "/status", "", http.StatusOK, `{
				"ok": true,
				"state": "failed",
				"error": "source unreachable"
			}`),
			want: Status{State: "failed", Error: "source unreachable"},
		},
		"500 returns APIError with extracted message": {
			handler: jsonHandler(t, http.MethodGet, "/status", "", http.StatusInternalServerError, `{
				"ok": false,
				"error": "boom"
			}`),
			wantAPI: &APIError{StatusCode: 500, Message: "boom"},
		},
		"malformed JSON propagates decode error": {
			handler:    jsonHandler(t, http.MethodGet, "/status", "", http.StatusOK, `not-json`),
			wantErrMsg: "decode /status response",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)

			c := NewClient(srv.URL)
			got, err := c.Status(t.Context())

			switch {
			case tc.wantAPI != nil:
				var apiErr *APIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tc.wantAPI.StatusCode, apiErr.StatusCode)
				assert.Equal(t, tc.wantAPI.Message, apiErr.Message)
			case tc.wantErrMsg != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestClient_Start(t *testing.T) {
	tests := map[string]struct {
		opts     StartOptions
		wantBody map[string]any
	}{
		"empty options omits all fields": {
			opts:     StartOptions{},
			wantBody: map[string]any{},
		},
		"exclude namespaces only": {
			opts:     StartOptions{ExcludeNamespaces: []string{"db1.coll1", "db2.*"}},
			wantBody: map[string]any{"excludeNamespaces": []any{"db1.coll1", "db2.*"}},
		},
		"both include and exclude": {
			opts: StartOptions{
				IncludeNamespaces: []string{"keep.*"},
				ExcludeNamespaces: []string{"skip.*"},
			},
			wantBody: map[string]any{
				"includeNamespaces": []any{"keep.*"},
				"excludeNamespaces": []any{"skip.*"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(bodyAssertingHandler(t, http.MethodPost, "/start", tc.wantBody))
			t.Cleanup(srv.Close)

			c := NewClient(srv.URL)
			require.NoError(t, c.Start(t.Context(), tc.opts))
		})
	}
}

func TestClient_SimplePOSTs(t *testing.T) {
	tests := map[string]struct {
		path string
		call func(context.Context, *Client) error
	}{
		"pause":    {path: "/pause", call: func(ctx context.Context, c *Client) error { return c.Pause(ctx) }},
		"reset":    {path: "/reset", call: func(ctx context.Context, c *Client) error { return c.Reset(ctx) }},
		"finalize": {path: "/finalize", call: func(ctx context.Context, c *Client) error { return c.Finalize(ctx) }},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(jsonHandler(t, http.MethodPost, tc.path, "", http.StatusOK, `{"ok":true}`))
			t.Cleanup(srv.Close)

			require.NoError(t, tc.call(t.Context(), NewClient(srv.URL)))
		})
	}
}

func TestClient_Resume(t *testing.T) {
	tests := map[string]struct {
		fromFailure bool
		wantBody    map[string]any
		wantNoBody  bool
	}{
		"clean resume sends no body": {
			fromFailure: false,
			wantNoBody:  true,
		},
		"from failure sends fromFailure=true": {
			fromFailure: true,
			wantBody:    map[string]any{"fromFailure": true},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var handler http.HandlerFunc
			if tc.wantNoBody {
				handler = func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, "/resume", r.URL.Path)
					raw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Empty(t, raw)
					_, _ = w.Write([]byte(`{"ok":true}`))
				}
			} else {
				handler = bodyAssertingHandler(t, http.MethodPost, "/resume", tc.wantBody)
			}
			srv := httptest.NewServer(handler)
			t.Cleanup(srv.Close)

			require.NoError(t, NewClient(srv.URL).Resume(t.Context(), tc.fromFailure))
		})
	}
}

func TestClient_APIErrorFallbackBody(t *testing.T) {
	// When PCSM returns a non-JSON error body, APIError.Message falls
	// back to the raw payload rather than swallowing it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("plain text failure"))
	}))
	t.Cleanup(srv.Close)

	err := NewClient(srv.URL).Pause(t.Context())
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.Equal(t, "plain text failure", apiErr.Message)
}

func TestAPIError_Error(t *testing.T) {
	tests := map[string]struct {
		err  APIError
		want string
	}{
		"with message": {
			err:  APIError{StatusCode: 500, Message: "boom"},
			want: "pcsm api: HTTP 500: boom",
		},
		"without message": {
			err:  APIError{StatusCode: 500},
			want: "pcsm api: HTTP 500",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}

func jsonHandler(t *testing.T, method, path, wantBody string, status int, response string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, method, r.Method)
		require.Equal(t, path, r.URL.Path)
		if wantBody != "" {
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.JSONEq(t, wantBody, string(raw))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}
}

func bodyAssertingHandler(t *testing.T, method, path string, want map[string]any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, method, r.Method)
		require.Equal(t, path, r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		got := map[string]any{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		assert.Equal(t, want, got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}
