// Copyright 2026 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPDumpRedactsBearerToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(httpDumpDirEnv, dir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		writeJSON(w, http.StatusOK, `[{"id":1,"uid":"prom","name":"Prometheus","type":"prometheus","url":"http://prometheus"}]`)
	}))
	defer server.Close()

	client := newTestClientWithToken(t, server.URL, "secret-token")
	_, err := client.ListDataSources(context.Background())
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	raw, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-token")

	var call dumpedCall
	require.NoError(t, json.Unmarshal(raw, &call))
	require.Equal(t, "GET", call.Method)
	require.Equal(t, "/api/datasources", call.Path)
	require.Equal(t, "Bearer <redacted>", call.Authorization)
	require.Equal(t, http.StatusOK, call.ResponseStatus)
	require.JSONEq(t, `[{"id":1,"uid":"prom","name":"Prometheus","type":"prometheus","url":"http://prometheus"}]`, string(call.ResponseBody))

	info, err := os.Stat(files[0])
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0), info.Mode().Perm()&0o077, "dump files must not be group/world-readable")
}

func TestHTTPDumpRedactsPermissionEmails(t *testing.T) {
	raw := []byte(`[{"userEmail":"user@example.com","userLogin":"user@example.com","userAvatarUrl":"/avatar/x","role":"Admin"}]`)
	got := redactDumpPII(dumpJSONBody(raw))
	require.NotContains(t, string(got), "user@example.com")
	var items []map[string]any
	require.NoError(t, json.Unmarshal(got, &items))
	require.Equal(t, "<redacted>", items[0]["userEmail"])
	require.Equal(t, "Admin", items[0]["role"])
}

func TestHTTPDumpRedactsRequestBodyEmails(t *testing.T) {
	raw := []byte(`{"title":"Scratchpad","userEmail":"user@example.com"}`)
	got := redactDumpPII(dumpJSONBody(raw))
	require.NotContains(t, string(got), "user@example.com")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(got, &payload))
	require.Equal(t, "Scratchpad", payload["title"])
	require.Equal(t, "<redacted>", payload["userEmail"])
}

func TestWrapDumpTransportNilBaseDoesNotPanic(t *testing.T) {
	rt := wrapDumpTransport(nil, &httpDumper{dir: t.TempDir()})
	require.NotNil(t, rt)
	dumper, ok := rt.(*dumpRoundTripper)
	require.True(t, ok)
	require.Equal(t, http.DefaultTransport, dumper.base)
}

func TestEnsureDumpDirTightensExistingPerms(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dumps")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, ensureDumpDir(dir))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(httpDumpDirPerm), info.Mode().Perm())
}

func TestHTTPDumperRecordReportsWriteError(t *testing.T) {
	d := &httpDumper{dir: filepath.Join(t.TempDir(), "missing")}
	req, err := http.NewRequest(http.MethodGet, "http://example/api/datasources", nil)
	require.NoError(t, err)
	require.Error(t, d.record(req, nil, http.StatusOK, []byte(`{}`)))
	require.Error(t, d.err())
}

func TestDumpJSONBodyKeepsFullArray(t *testing.T) {
	raw := make([]byte, 0, 8192)
	raw = append(raw, '[')
	for i := 0; i < 600; i++ {
		if i > 0 {
			raw = append(raw, ',')
		}
		raw = append(raw, `{"id":1}`...)
	}
	raw = append(raw, ']')

	got := dumpJSONBody(raw)
	require.JSONEq(t, string(raw), string(got))
	require.NotContains(t, string(got), `"_truncated"`)
}

func TestDumpJSONBodyNonJSONIsMarshalable(t *testing.T) {
	got := dumpJSONBody([]byte("upstream gateway timeout user@example.com"))
	require.True(t, json.Valid(got), string(got))
	require.NotContains(t, string(got), "user@example.com")
	var wrap struct {
		NonJSON bool `json:"_nonJSON"`
		Bytes   int  `json:"bytes"`
	}
	require.NoError(t, json.Unmarshal(got, &wrap))
	require.True(t, wrap.NonJSON)
	require.Equal(t, len("upstream gateway timeout user@example.com"), wrap.Bytes)

	call := dumpedCall{ResponseBody: got}
	_, err := json.Marshal(call)
	require.NoError(t, err)
}

type dumpRoundTripFunc func(*http.Request) (*http.Response, error)

func (f dumpRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDumpRoundTripperPreservesGetBodyForRetries(t *testing.T) {
	var seen *http.Request
	inner := dumpRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Header:     make(http.Header),
		}, nil
	})
	rt := wrapDumpTransport(inner, &httpDumper{dir: t.TempDir()})
	req, err := http.NewRequest(http.MethodPost, "http://example/api/folders", bytes.NewReader([]byte(`{"title":"x"}`)))
	require.NoError(t, err)
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, seen.GetBody)
	replay, err := seen.GetBody()
	require.NoError(t, err)
	defer func() { _ = replay.Close() }()
	raw, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, `{"title":"x"}`, string(raw))
}
