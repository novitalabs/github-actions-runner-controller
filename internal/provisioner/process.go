package provisioner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type request struct {
	ID           int            `json:"id"`
	Op           string         `json:"op"`
	ControllerID string         `json:"controller_id,omitempty"`
	ScaleSetID   int            `json:"scale_set_id,omitempty"`
	SandboxID    string         `json:"sandbox_id,omitempty"`
	Create       *CreateRequest `json:"create,omitempty"`
}
type response struct {
	ID      int      `json:"id"`
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
	Runner  *Runner  `json:"runner,omitempty"`
	Runners []Runner `json:"runners,omitempty"`
}
type Process struct {
	mu   sync.Mutex
	next int
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
}

func NewProcess(ctx context.Context, python, path string) (*Process, error) {
	c := exec.CommandContext(ctx, python, "-u", path)
	in, e := c.StdinPipe()
	if e != nil {
		return nil, e
	}
	out, e := c.StdoutPipe()
	if e != nil {
		return nil, e
	}
	c.Stderr = nil
	if e = c.Start(); e != nil {
		return nil, e
	}
	return &Process{cmd: c, in: in, out: bufio.NewReader(out)}, nil
}
func (p *Process) call(req request, res *response) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next++
	req.ID = p.next
	b, e := json.Marshal(req)
	if e != nil {
		return e
	}
	if _, e = p.in.Write(append(b, '\n')); e != nil {
		return e
	}
	line, e := p.out.ReadBytes('\n')
	if e != nil {
		return e
	}
	if e = json.Unmarshal(line, res); e != nil {
		return fmt.Errorf("invalid adapter response: %w", e)
	}
	if res.ID != req.ID {
		return fmt.Errorf("adapter response id mismatch")
	}
	if !res.OK {
		return fmt.Errorf("novita adapter: %s", res.Error)
	}
	return nil
}
func (p *Process) List(_ context.Context, c string, s int) ([]Runner, error) {
	var r response
	e := p.call(request{Op: "list", ControllerID: c, ScaleSetID: s}, &r)
	return r.Runners, e
}
func (p *Process) Create(_ context.Context, q CreateRequest) (Runner, error) {
	var r response
	e := p.call(request{Op: "create", Create: &q}, &r)
	if e != nil {
		return Runner{}, e
	}
	if r.Runner == nil {
		return Runner{}, fmt.Errorf("adapter omitted runner")
	}
	return *r.Runner, nil
}
func (p *Process) Delete(_ context.Context, id string) error {
	var r response
	return p.call(request{Op: "delete", SandboxID: id}, &r)
}
func (p *Process) Close() error { _ = p.in.Close(); return p.cmd.Wait() }
