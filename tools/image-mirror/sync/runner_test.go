package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
)

func testLogger() logr.Logger {
	return funcr.New(func(prefix, args string) {
		// discard for tests
	}, funcr.Options{})
}

func TestReadBuildTag(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantTag string
		wantErr bool
	}{
		{
			name:    "valid metadata",
			content: `{"build_tag": "v1.0.0-abc123"}`,
			wantTag: "v1.0.0-abc123",
		},
		{
			name:    "missing build_tag",
			content: `{"other": "value"}`,
			wantErr: true,
		},
		{
			name:    "empty build_tag",
			content: `{"build_tag": ""}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			content: `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			metadataFile := filepath.Join(tmpDir, "metadata.json")
			if err := os.WriteFile(metadataFile, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			r := &Runner{logger: testLogger()}
			tag, err := r.readBuildTag(metadataFile)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tag != tt.wantTag {
				t.Errorf("expected tag %q, got %q", tt.wantTag, tag)
			}
		})
	}
}

func TestReadBuildTag_FileNotFound(t *testing.T) {
	r := &Runner{logger: testLogger()}
	_, err := r.readBuildTag("/nonexistent/path/metadata.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestDownloadFromSAS(t *testing.T) {
	expectedContent := "test file content for image tar"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, expectedContent)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "subdir", "downloaded.tar")

	r := &Runner{
		opts:   &Options{completedOptions: &completedOptions{}},
		logger: testLogger(),
	}

	ctx := context.Background()
	err := r.downloadFromSAS(ctx, server.URL, destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, string(data))
	}
}

func TestDownloadFromSAS_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.tar")

	r := &Runner{
		opts:   &Options{completedOptions: &completedOptions{}},
		logger: testLogger(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := r.downloadFromSAS(ctx, server.URL, destPath)
	if err == nil {
		t.Fatal("expected error for HTTP 403, got nil")
	}
}

func TestDownloadFromSAS_ContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay so the context can cancel first
		time.Sleep(2 * time.Second)
		_, _ = fmt.Fprint(w, "should not complete")
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.tar")

	r := &Runner{
		opts:   &Options{completedOptions: &completedOptions{}},
		logger: testLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := r.downloadFromSAS(ctx, server.URL, destPath)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestResolveImageFiles_LocalFiles(t *testing.T) {
	tmpDir := t.TempDir()

	tarFile := filepath.Join(tmpDir, "image.tar")
	metadataFile := filepath.Join(tmpDir, "metadata.json")
	if err := os.WriteFile(tarFile, []byte("tar content"), 0o644); err != nil {
		t.Fatalf("failed to write tar file: %v", err)
	}
	if err := os.WriteFile(metadataFile, []byte(`{"build_tag":"v1"}`), 0o644); err != nil {
		t.Fatalf("failed to write metadata file: %v", err)
	}

	r := &Runner{
		opts: &Options{completedOptions: &completedOptions{
			ImageFilePath:         tmpDir,
			ImageTarFileName:      "image.tar",
			ImageMetadataFileName: "metadata.json",
		}},
		logger: testLogger(),
	}

	ctx := context.Background()
	gotTar, gotMeta, err := r.resolveImageFiles(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTar != tarFile {
		t.Errorf("expected tar file %q, got %q", tarFile, gotTar)
	}
	if gotMeta != metadataFile {
		t.Errorf("expected metadata file %q, got %q", metadataFile, gotMeta)
	}
}

func TestResolveImageFiles_MissingTarFile(t *testing.T) {
	tmpDir := t.TempDir()

	metadataFile := filepath.Join(tmpDir, "metadata.json")
	if err := os.WriteFile(metadataFile, []byte(`{"build_tag":"v1"}`), 0o644); err != nil {
		t.Fatalf("failed to write metadata file: %v", err)
	}

	r := &Runner{
		opts: &Options{completedOptions: &completedOptions{
			ImageFilePath:         tmpDir,
			ImageTarFileName:      "image.tar",
			ImageMetadataFileName: "metadata.json",
		}},
		logger: testLogger(),
	}

	ctx := context.Background()
	_, _, err := r.resolveImageFiles(ctx)
	if err == nil {
		t.Fatal("expected error for missing tar file, got nil")
	}
}

func TestResolveImageFiles_SASDownload(t *testing.T) {
	tarContent := "fake tar content"
	metadataContent, _ := json.Marshal(map[string]string{"build_tag": "v1.0.0"})

	mux := http.NewServeMux()
	mux.HandleFunc("/image.tar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, tarContent)
	})
	mux.HandleFunc("/metadata.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(metadataContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	tmpDir := t.TempDir()

	r := &Runner{
		opts: &Options{completedOptions: &completedOptions{
			ImageFilePath:         tmpDir,
			ImageTarFileName:      "image.tar",
			ImageMetadataFileName: "metadata.json",
			ImageTarSAS:           server.URL + "/image.tar",
			ImageMetadataSAS:      server.URL + "/metadata.json",
		}},
		logger: testLogger(),
	}

	ctx := context.Background()
	gotTar, gotMeta, err := r.resolveImageFiles(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify files were downloaded
	data, err := os.ReadFile(gotTar)
	if err != nil {
		t.Fatalf("failed to read downloaded tar: %v", err)
	}
	if string(data) != tarContent {
		t.Errorf("expected tar content %q, got %q", tarContent, string(data))
	}

	data, err = os.ReadFile(gotMeta)
	if err != nil {
		t.Fatalf("failed to read downloaded metadata: %v", err)
	}
	if string(data) != string(metadataContent) {
		t.Errorf("expected metadata content %q, got %q", string(metadataContent), string(data))
	}
}

func TestRetry_Success(t *testing.T) {
	attempts := 0
	err := retry(context.Background(), 3, func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("not yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_AllFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	attempts := 0
	err := retry(ctx, 3, func() error {
		attempts++
		return fmt.Errorf("always fail")
	})
	if err == nil {
		t.Fatal("expected error after all retries, got nil")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	attempts := 0
	err := retry(ctx, 5, func() error {
		attempts++
		return fmt.Errorf("fail")
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	// Should have done 1 attempt, then context was cancelled before the sleep
	if attempts != 1 {
		t.Errorf("expected 1 attempt before context cancel, got %d", attempts)
	}
}

func TestRetry_ImmediateSuccess(t *testing.T) {
	attempts := 0
	err := retry(context.Background(), 5, func() error {
		attempts++
		return nil
	})
	if err != nil {
		t.Fatalf("expected immediate success, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}
