package provisioner

import (
	"context"
	"time"
)

type Runner struct {
	Name      string    `json:"name"`
	SandboxID string    `json:"sandbox_id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}
type CreateRequest struct {
	Name           string `json:"name"`
	JITConfig      string `json:"jit_config"`
	Template       string `json:"template"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ControllerID   string `json:"controller_id"`
	ScaleSetID     int    `json:"scale_set_id"`
}
type Provisioner interface {
	List(context.Context, string, int) ([]Runner, error)
	Create(context.Context, CreateRequest) (Runner, error)
	Delete(context.Context, string) error
	Close() error
}
