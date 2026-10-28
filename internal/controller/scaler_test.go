package controller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/garysng/novita-actions-runner-controller/internal/provisioner"
	"github.com/google/uuid"
)

type fakeGitHub struct{ configs []string }

func (f *fakeGitHub) GenerateJitRunnerConfig(_ context.Context, s *scaleset.RunnerScaleSetJitRunnerSetting, _ int) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
	f.configs = append(f.configs, "TOP-SECRET-JIT")
	return &scaleset.RunnerScaleSetJitRunnerConfig{Runner: &scaleset.RunnerReference{Name: s.Name}, EncodedJITConfig: "TOP-SECRET-JIT"}, nil
}

type fakeSession struct{ acquired [][]int64 }

func (f *fakeSession) GetMessage(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
	return nil, nil
}
func (f *fakeSession) DeleteMessage(context.Context, int) error { return nil }
func (f *fakeSession) AcquireJobs(_ context.Context, ids []int64) ([]int64, error) {
	f.acquired = append(f.acquired, ids)
	return ids, nil
}
func (f *fakeSession) Session() scaleset.RunnerScaleSetSession {
	return scaleset.RunnerScaleSetSession{SessionID: uuid.New()}
}

type fakeProvisioner struct {
	runners    []provisioner.Runner
	creates    []provisioner.CreateRequest
	deletes    []string
	failCreate bool
}

func (f *fakeProvisioner) List(context.Context, string, int) ([]provisioner.Runner, error) {
	return append([]provisioner.Runner(nil), f.runners...), nil
}
func (f *fakeProvisioner) Create(_ context.Context, q provisioner.CreateRequest) (provisioner.Runner, error) {
	f.creates = append(f.creates, q)
	if f.failCreate {
		return provisioner.Runner{}, errors.New("boom")
	}
	for _, r := range f.runners {
		if r.Name == q.Name {
			return r, nil
		}
	}
	r := provisioner.Runner{Name: q.Name, SandboxID: q.Name, State: "running", CreatedAt: time.Now()}
	f.runners = append(f.runners, r)
	return r, nil
}
func (f *fakeProvisioner) Delete(_ context.Context, id string) error {
	f.deletes = append(f.deletes, id)
	for i, r := range f.runners {
		if r.SandboxID == id {
			f.runners = append(f.runners[:i], f.runners[i+1:]...)
			break
		}
	}
	return nil
}
func (f *fakeProvisioner) Close() error { return nil }
func newScaler(p *fakeProvisioner) (*Scaler, *fakeGitHub, *fakeSession) {
	g := &fakeGitHub{}
	s := &fakeSession{}
	return &Scaler{GitHub: g, Session: s, Provisioner: p, ScaleSetID: 7, ControllerID: "test", Template: "runner", Max: 4, Timeout: time.Hour, OrphanGrace: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, g, s
}

func TestDuplicateMessageDoesNotDuplicateRunner(t *testing.T) {
	p := &fakeProvisioner{}
	s, _, session := newScaler(p)
	m := &scaleset.RunnerScaleSetMessage{JobAvailableMessages: []*scaleset.JobAvailable{{JobMessageBase: scaleset.JobMessageBase{RunnerRequestID: 42}}}, Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}}
	if err := s.Scale(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if err := s.Scale(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if len(p.runners) != 1 {
		t.Fatalf("got %d runners", len(p.runners))
	}
	if len(session.acquired) != 2 {
		t.Fatalf("expected safe re-acquire")
	}
}
func TestPartialScaleFailureReusesRunnerName(t *testing.T) {
	p := &fakeProvisioner{failCreate: true}
	s, _, _ := newScaler(p)
	m := &scaleset.RunnerScaleSetMessage{Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}}
	_ = s.Scale(t.Context(), m)
	first := p.creates[0].Name
	p.failCreate = false
	if err := s.Scale(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if p.creates[1].Name != first {
		t.Fatalf("retry changed name")
	}
}
func TestBusyAndRunningRunnersAreNotOrphaned(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	p := &fakeProvisioner{runners: []provisioner.Runner{{Name: "busy", SandboxID: "b", State: "paused", CreatedAt: old}, {Name: "unknown-running", SandboxID: "r", State: "running", CreatedAt: old}}}
	s, _, _ := newScaler(p)
	s.busy = map[string]bool{"busy": true}
	if err := s.Scale(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if len(p.deletes) != 0 {
		t.Fatalf("deleted protected runner: %v", p.deletes)
	}
}
func TestPausedOrphanIsDeleted(t *testing.T) {
	p := &fakeProvisioner{runners: []provisioner.Runner{{Name: "orphan", SandboxID: "o", State: "paused", CreatedAt: time.Now().Add(-time.Hour)}}}
	s, _, _ := newScaler(p)
	if err := s.Scale(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if len(p.deletes) != 1 || p.deletes[0] != "o" {
		t.Fatalf("deletes=%v", p.deletes)
	}
}
func TestCompletionIsIdempotent(t *testing.T) {
	p := &fakeProvisioner{runners: []provisioner.Runner{{Name: "done", SandboxID: "x", State: "running"}}}
	s, _, _ := newScaler(p)
	m := &scaleset.RunnerScaleSetMessage{JobCompletedMessages: []*scaleset.JobCompleted{{RunnerName: "done"}}}
	if err := s.Scale(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if err := s.Scale(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if len(p.deletes) != 1 {
		t.Fatalf("deletes=%v", p.deletes)
	}
}

func TestCompletionWithoutStatisticsDoesNotReplaceFinishedRunner(t *testing.T) {
	p := &fakeProvisioner{runners: []provisioner.Runner{{Name: "done", SandboxID: "x", State: "running"}}}
	s, _, _ := newScaler(p)
	s.assigned = 1
	m := &scaleset.RunnerScaleSetMessage{JobCompletedMessages: []*scaleset.JobCompleted{{RunnerName: "done"}}}
	if err := s.Scale(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if len(p.runners) != 0 || len(p.creates) != 0 {
		t.Fatalf("completion incorrectly replaced runner: runners=%v creates=%v", p.runners, p.creates)
	}
}
func TestLogsDoNotContainJIT(t *testing.T) {
	var out strings.Builder
	p := &fakeProvisioner{}
	s, _, _ := newScaler(p)
	s.Log = slog.New(slog.NewTextHandler(&out, nil))
	if err := s.Scale(t.Context(), &scaleset.RunnerScaleSetMessage{Statistics: &scaleset.RunnerScaleSetStatistic{TotalAssignedJobs: 1}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "TOP-SECRET-JIT") {
		t.Fatal("JIT leaked to logs")
	}
}
