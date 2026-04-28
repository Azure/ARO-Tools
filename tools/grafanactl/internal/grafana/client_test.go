// Copyright 2025 Microsoft Corporation
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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana-tools/sdk"
)

func TestCreateDataSourceUsesStatusAwareRequest(t *testing.T) {
	var gotAuthHeader string
	var gotDatasource sdk.Datasource

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected method %s, got %s", http.MethodPost, r.Method)
		}
		if r.URL.Path != "/api/datasources" {
			t.Fatalf("expected path /api/datasources, got %s", r.URL.Path)
		}
		gotAuthHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotDatasource); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":"created"}`))
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		token:      "token",
		httpClient: server.Client(),
	}

	err := client.CreateDataSource(context.Background(), sdk.Datasource{
		Name: "kusto-int-uksouth",
		Type: "grafana-azure-data-explorer-datasource",
	})
	if err != nil {
		t.Fatalf("CreateDataSource returned error: %v", err)
	}
	if gotAuthHeader != "Bearer token" {
		t.Fatalf("expected bearer token auth header, got %q", gotAuthHeader)
	}
	if gotDatasource.Name != "kusto-int-uksouth" {
		t.Fatalf("expected datasource name in request, got %q", gotDatasource.Name)
	}
}

func TestUpdateDataSourceReportsForbiddenAsAdminPermissionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()

	client := &Client{
		endpoint:   server.URL,
		token:      "token",
		httpClient: server.Client(),
	}

	err := client.UpdateDataSource(context.Background(), sdk.Datasource{
		ID:   42,
		Name: "kusto-int-uksouth",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Grafana Admin permissions") {
		t.Fatalf("expected Grafana Admin permissions error, got %v", err)
	}
}
