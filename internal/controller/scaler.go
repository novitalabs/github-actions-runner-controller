package controller

import (
	"context"
	"fmt"
	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/garysng/novita-actions-runner-controller/internal/provisioner"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type GitHub interface {
	GenerateJitRunnerConfig(context.Context, *scaleset.RunnerScaleSetJitRunnerSetting, int) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
}
type Scaler struct {
	GitHub                   GitHub
	Session                  listener.Client
	Provisioner              provisioner.Provisioner
	ScaleSetID               int
	ControllerID, Template   string
	Min, Max                 int
	Timeout, OrphanGrace     time.Duration
	Log                      *slog.Logger
	mu                       sync.Mutex
	busy                     map[string]bool
	pending                  string
	assigned                 int
	created, deleted, errors atomic.Uint64
}

func (s *Scaler) Scale(ctx context.Context, m *scaleset.RunnerScaleSetMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy == nil {
		s.busy = map[string]bool{}
	}
	if m != nil && len(m.JobAvailableMessages) > 0 {
		ids := make([]int64, 0, len(m.JobAvailableMessages))
		for _, j := range m.JobAvailableMessages {
			ids = append(ids, j.RunnerRequestID)
		}
		if _, e := s.Session.AcquireJobs(ctx, ids); e != nil {
			return fmt.Errorf("acquire jobs: %w", e)
		}
	}
	if m != nil {
		for _, j := range m.JobStartedMessages {
			s.busy[j.RunnerName] = true
		}
		for _, j := range m.JobCompletedMessages {
			if e := s.deleteByName(ctx, j.RunnerName); e != nil {
				return e
			}
			delete(s.busy, j.RunnerName)
			if m.Statistics == nil && s.assigned > 0 {
				s.assigned--
			}
		}
		if m.Statistics != nil {
			s.assigned = m.Statistics.TotalAssignedJobs
		}
	}
	return s.reconcile(ctx)
}
func (s *Scaler) reconcile(ctx context.Context) error {
	rs, e := s.Provisioner.List(ctx, s.ControllerID, s.ScaleSetID)
	if e != nil {
		s.errors.Add(1)
		return e
	}
	now := time.Now()
	for _, r := range rs {
		// A running runner may be executing a job whose JobStarted message was
		// observed by a previous controller process. Only paused sandboxes are
		// safe orphan candidates after a restart.
		if r.State == "paused" && !s.busy[r.Name] && !r.CreatedAt.IsZero() && now.Sub(r.CreatedAt) > s.OrphanGrace {
			if e := s.Provisioner.Delete(ctx, r.SandboxID); e != nil {
				return e
			}
			s.deleted.Add(1)
		}
	}
	rs, e = s.Provisioner.List(ctx, s.ControllerID, s.ScaleSetID)
	if e != nil {
		return e
	}
	target := s.Min + s.assigned
	if target > s.Max {
		target = s.Max
	}
	for len(rs) < target {
		name := s.pending
		if name == "" {
			name = fmt.Sprintf("novita-%d-%d", s.ScaleSetID, time.Now().UnixNano())
			s.pending = name
		}
		jit, e := s.GitHub.GenerateJitRunnerConfig(ctx, &scaleset.RunnerScaleSetJitRunnerSetting{Name: name, WorkFolder: "_work"}, s.ScaleSetID)
		if e != nil {
			return e
		}
		r, e := s.Provisioner.Create(ctx, provisioner.CreateRequest{Name: name, JITConfig: jit.EncodedJITConfig, Template: s.Template, TimeoutSeconds: int(s.Timeout.Seconds()), ControllerID: s.ControllerID, ScaleSetID: s.ScaleSetID})
		if e != nil {
			s.errors.Add(1)
			return e
		}
		s.pending = ""
		s.created.Add(1)
		rs = append(rs, r)
	}
	return nil
}
func (s *Scaler) deleteByName(ctx context.Context, name string) error {
	rs, e := s.Provisioner.List(ctx, s.ControllerID, s.ScaleSetID)
	if e != nil {
		return e
	}
	for _, r := range rs {
		if r.Name == name {
			if e := s.Provisioner.Delete(ctx, r.SandboxID); e != nil {
				return e
			}
			s.deleted.Add(1)
			return nil
		}
	}
	return nil
}
func (s *Scaler) Metrics() (uint64, uint64, uint64) {
	return s.created.Load(), s.deleted.Load(), s.errors.Load()
}

var _ listener.Scaler = (*Scaler)(nil)
