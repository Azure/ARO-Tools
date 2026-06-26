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

package prowjob

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/protobuf/encoding/protojson"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowjobs "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
	prowgangway "sigs.k8s.io/prow/pkg/gangway"
	"sigs.k8s.io/yaml"
)

const testJobName = "branch-ci-Azure-ARO-HCP-main-e2e-integration-e2e-parallel"

// abortFixture is a test Prow/Gangway server. It serves ProwJob status (the Deck
// "?prowjob=<id>" endpoint), the Gangway ListJobExecutions endpoint
// ("?job_name=<>&status=<>"), and records any bulk status-change (abort) requests.
type abortFixture struct {
	srv       *httptest.Server
	jobs      map[string]*prowjobs.ProwJob
	bulkCount int32
	mu        sync.Mutex
	lastBulk  *prowgangway.BulkJobStatusChangeRequest
}

func newAbortFixture(t *testing.T, jobs map[string]*prowjobs.ProwJob) *abortFixture {
	t.Helper()
	f := &abortFixture{jobs: jobs}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == bulkStatusChangePath {
			atomic.AddInt32(&f.bulkCount, 1)
			raw, _ := io.ReadAll(r.Body)
			var parsed prowgangway.BulkJobStatusChangeRequest
			if err := protojson.Unmarshal(raw, &parsed); err == nil {
				f.mu.Lock()
				f.lastBulk = &parsed
				f.mu.Unlock()
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}

		q := r.URL.Query()
		if id := q.Get("prowjob"); id != "" {
			job, ok := f.jobs[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			body, err := yaml.Marshal(job)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
			return
		}

		if jobName := q.Get("job_name"); jobName != "" {
			var want prowgangway.JobExecutionStatus
			if name := q.Get("status"); name != "" {
				want = prowgangway.JobExecutionStatus(prowgangway.JobExecutionStatus_value[name])
			}
			execs := &prowgangway.JobExecutions{}
			for id, j := range f.jobs {
				if j.Spec.Job != jobName {
					continue
				}
				st := prowgangway.TranslateProwJobStatus(&j.Status)
				if want != prowgangway.JobExecutionStatus_JOB_EXECUTION_STATUS_UNSPECIFIED && st != want {
					continue
				}
				execs.JobExecution = append(execs.JobExecution, &prowgangway.JobExecution{
					Id:        id,
					JobName:   j.Spec.Job,
					JobStatus: st,
					JobType:   prowgangway.TranslateProwJobType(j.Spec.Type),
				})
			}
			out, err := protojson.Marshal(execs)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(out)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *abortFixture) client() *Client {
	return NewClient("test-token", f.srv.URL, f.srv.URL)
}

func (f *abortFixture) bulkRequests() int32 {
	return atomic.LoadInt32(&f.bulkCount)
}

func testJob(state prowjobs.ProwJobState, jobType prowjobs.ProwJobType, refs *prowjobs.Refs, start time.Time, region string) *prowjobs.ProwJob {
	pj := &prowjobs.ProwJob{
		Spec: prowjobs.ProwJobSpec{
			Job:  testJobName,
			Type: jobType,
			Refs: refs,
		},
		Status: prowjobs.ProwJobStatus{
			State: state,
			URL:   "https://prow.ci.openshift.org/view/job",
		},
	}
	if region != "" {
		pj.Annotations = map[string]string{ev2RolloutRegionAnnotation: region}
	}
	if !start.IsZero() {
		pj.Status.StartTime = metav1.NewTime(start)
	}
	return pj
}

func postsubmitRefs() *prowjobs.Refs {
	return &prowjobs.Refs{Org: "Azure", Repo: "ARO-HCP", BaseRef: "main", BaseSHA: "deadbeef"}
}

func TestDeriveBulkURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "executions endpoint",
			in:   "https://gangway-ci.example.com/v1/executions",
			want: "https://gangway-ci.example.com/v1/bulk-job-status-update",
		},
		{
			name: "strips query",
			in:   "https://gangway-ci.example.com/v1/executions?foo=bar",
			want: "https://gangway-ci.example.com/v1/bulk-job-status-update",
		},
		{
			name: "host only",
			in:   "http://127.0.0.1:8080",
			want: "http://127.0.0.1:8080/v1/bulk-job-status-update",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveBulkURL(tc.in); got != tc.want {
				t.Fatalf("deriveBulkURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsTerminalState(t *testing.T) {
	terminal := []prowjobs.ProwJobState{
		prowjobs.SuccessState, prowjobs.FailureState, prowjobs.AbortedState, prowjobs.ErrorState,
	}
	for _, s := range terminal {
		if !isTerminalState(s) {
			t.Errorf("isTerminalState(%q) = false, want true", s)
		}
	}
	nonTerminal := []prowjobs.ProwJobState{
		prowjobs.TriggeredState, prowjobs.PendingState, prowjobs.SchedulingState,
	}
	for _, s := range nonTerminal {
		if isTerminalState(s) {
			t.Errorf("isTerminalState(%q) = true, want false", s)
		}
	}
}

func TestAbortJobRunningJob(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"job-exec-123": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "eastus"),
	})

	if err := f.client().AbortJob(testContext(), "job-exec-123"); err != nil {
		t.Fatalf("AbortJob returned error: %v", err)
	}

	if got := f.bulkRequests(); got != 1 {
		t.Fatalf("expected exactly 1 bulk request, got %d", got)
	}

	f.mu.Lock()
	req := f.lastBulk
	f.mu.Unlock()
	if req == nil {
		t.Fatal("bulk request body was not captured")
	}
	if req.GetJobStatusChange().GetCurrent() != prowgangway.JobExecutionStatus_PENDING {
		t.Errorf("current = %v, want PENDING", req.GetJobStatusChange().GetCurrent())
	}
	if req.GetJobStatusChange().GetDesired() != prowgangway.JobExecutionStatus_ABORTED {
		t.Errorf("desired = %v, want ABORTED", req.GetJobStatusChange().GetDesired())
	}
	if req.GetJobType() != prowgangway.JobExecutionType_POSTSUBMIT {
		t.Errorf("jobType = %v, want POSTSUBMIT", req.GetJobType())
	}
	if req.GetRefs().GetOrg() != "Azure" || req.GetRefs().GetRepo() != "ARO-HCP" {
		t.Errorf("refs = %v, want org=Azure repo=ARO-HCP", req.GetRefs())
	}
	if !req.GetStartedAfter().AsTime().Equal(start) || !req.GetStartedBefore().AsTime().Equal(start) {
		t.Errorf("window = [%v, %v], want both = %v", req.GetStartedAfter().AsTime(), req.GetStartedBefore().AsTime(), start)
	}
}

func TestAbortJobTerminalIsNoop(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"job-exec-123": testJob(prowjobs.SuccessState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "eastus"),
	})

	if err := f.client().AbortJob(testContext(), "job-exec-123"); err != nil {
		t.Fatalf("AbortJob returned error: %v", err)
	}
	if got := f.bulkRequests(); got != 0 {
		t.Fatalf("expected no bulk request for terminal job, got %d", got)
	}
}

func TestAbortJobNoStartTimeIsNoop(t *testing.T) {
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"job-exec-123": testJob(prowjobs.TriggeredState, prowjobs.PostsubmitJob, postsubmitRefs(), time.Time{}, "eastus"),
	})

	if err := f.client().AbortJob(testContext(), "job-exec-123"); err != nil {
		t.Fatalf("AbortJob returned error: %v", err)
	}
	if got := f.bulkRequests(); got != 0 {
		t.Fatalf("expected no bulk request when start time is unknown, got %d", got)
	}
}

// TestAbortJobSkipsWhenRegionSharesWindow verifies that when another region's
// execution of the same job started in the same second (and is therefore
// indistinguishable to the region-blind, second-precise bulk API), AbortJob
// refuses to fire rather than cancel the sibling region's job.
func TestAbortJobSkipsWhenRegionSharesWindow(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"exec-eastus": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "eastus"),
		"exec-westus": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "westus"),
	})

	if err := f.client().AbortJob(testContext(), "exec-eastus"); err != nil {
		t.Fatalf("AbortJob returned error: %v", err)
	}
	if got := f.bulkRequests(); got != 0 {
		t.Fatalf("expected no bulk request when a sibling region shares the window, got %d", got)
	}
}

// TestAbortJobAbortsWhenRegionsDifferInTime verifies that concurrent regional
// executions which started in different seconds do not block the abort: the
// target is isolable, so exactly one bulk request is issued for our window.
func TestAbortJobAbortsWhenRegionsDifferInTime(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"exec-eastus": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "eastus"),
		"exec-westus": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start.Add(5*time.Second), "westus"),
	})

	if err := f.client().AbortJob(testContext(), "exec-eastus"); err != nil {
		t.Fatalf("AbortJob returned error: %v", err)
	}
	if got := f.bulkRequests(); got != 1 {
		t.Fatalf("expected exactly 1 bulk request, got %d", got)
	}

	f.mu.Lock()
	req := f.lastBulk
	f.mu.Unlock()
	if req == nil {
		t.Fatal("bulk request body was not captured")
	}
	if !req.GetStartedAfter().AsTime().Equal(start) {
		t.Errorf("window anchored at %v, want %v (the target region's start)", req.GetStartedAfter().AsTime(), start)
	}
}

// waitCtx wraps a discard-logger context with a cancel function.
func waitCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(logr.NewContext(context.Background(), logr.Discard()))
}

func TestWaitForCompletionAbortsOnCancel(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"job-exec-123": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "eastus"),
	})

	m := NewMonitor(f.client(), 5*time.Millisecond, time.Hour, false, true, true)

	ctx, cancel := waitCtx()
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.WaitForCompletion(ctx, logr.Discard(), "job-exec-123")
	}()

	// Let the monitor observe the job at least once, then cancel.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error on cancellation, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForCompletion did not return after cancellation")
	}

	if got := f.bulkRequests(); got != 1 {
		t.Fatalf("expected exactly 1 abort request, got %d", got)
	}
}

func TestWaitForCompletionNoAbortWhenDisabled(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"job-exec-123": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "eastus"),
	})

	m := NewMonitor(f.client(), 5*time.Millisecond, time.Hour, false, true, false)

	ctx, cancel := waitCtx()
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.WaitForCompletion(ctx, logr.Discard(), "job-exec-123")
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForCompletion did not return after cancellation")
	}

	if got := f.bulkRequests(); got != 0 {
		t.Fatalf("expected no abort request when abort-on-cancel is disabled, got %d", got)
	}
}

func TestWaitForCompletionTimeoutDoesNotAbort(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	f := newAbortFixture(t, map[string]*prowjobs.ProwJob{
		"job-exec-123": testJob(prowjobs.PendingState, prowjobs.PostsubmitJob, postsubmitRefs(), start, "eastus"),
	})

	// Short monitor timeout, parent context never cancelled: this is an internal
	// timeout, which must not abort the job.
	m := NewMonitor(f.client(), 5*time.Millisecond, 20*time.Millisecond, false, true, true)

	err := m.WaitForCompletion(testContext(), logr.Discard(), "job-exec-123")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if got := f.bulkRequests(); got != 0 {
		t.Fatalf("expected no abort request on internal timeout, got %d", got)
	}
}
