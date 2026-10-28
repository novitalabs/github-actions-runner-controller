package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/novitalabs/github-actions-runner-controller/internal/config"
	"github.com/novitalabs/github-actions-runner-controller/internal/controller"
	githubhttp "github.com/novitalabs/github-actions-runner-controller/internal/github"
	"github.com/novitalabs/github-actions-runner-controller/internal/provisioner"
	"github.com/novitalabs/github-actions-runner-controller/internal/telemetry"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx); err != nil {
		slog.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client, err := scaleset.NewClientWithPersonalAccessToken(scaleset.NewClientWithPersonalAccessTokenConfig{
		GitHubConfigURL: cfg.GitHubURL, PersonalAccessToken: cfg.GitHubToken,
		SystemInfo: scaleset.SystemInfo{System: "novita-actions-runner-controller", Version: version, Subsystem: "controller"},
	}, scaleset.WithHTTPClient(githubhttp.IdentityEncodingClient()))
	if err != nil {
		return fmt.Errorf("github client: %w", err)
	}
	groupID := 1
	if cfg.RunnerGroup != scaleset.DefaultRunnerGroup {
		g, e := client.GetRunnerGroupByName(ctx, cfg.RunnerGroup)
		if e != nil {
			return e
		}
		groupID = g.ID
	}
	scaleSet, err := client.GetRunnerScaleSet(ctx, groupID, cfg.ScaleSetName)
	if err != nil {
		return err
	}
	labels := make([]scaleset.Label, 0, len(cfg.Labels))
	for _, v := range cfg.Labels {
		labels = append(labels, scaleset.Label{Name: v})
	}
	if scaleSet == nil {
		scaleSet, err = client.CreateRunnerScaleSet(ctx, &scaleset.RunnerScaleSet{
			Name: cfg.ScaleSetName, RunnerGroupID: groupID, Labels: labels,
			RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		})
	} else {
		scaleSet, err = client.UpdateRunnerScaleSet(ctx, scaleSet.ID, &scaleset.RunnerScaleSet{
			Name: scaleSet.Name, RunnerGroupID: groupID, Labels: labels,
			RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
		})
	}
	if err != nil {
		return fmt.Errorf("ensure scale set: %w", err)
	}
	client.SetSystemInfo(scaleset.SystemInfo{System: "novita-actions-runner-controller", Version: version, Subsystem: "controller", ScaleSetID: scaleSet.ID})
	session, err := client.MessageSessionClient(ctx, scaleSet.ID, cfg.ControllerID)
	if err != nil {
		return err
	}
	defer session.Close(context.Background())
	adapter, err := provisioner.NewProcess(ctx, cfg.Python, cfg.Adapter)
	if err != nil {
		return err
	}
	defer adapter.Close()
	scaler := &controller.Scaler{GitHub: client, Session: session, Provisioner: adapter, ScaleSetID: scaleSet.ID, ControllerID: cfg.ControllerID, Template: cfg.RunnerTemplate, Min: cfg.MinRunners, Max: cfg.MaxRunners, Timeout: cfg.RunnerTimeout, OrphanGrace: cfg.OrphanGrace, Log: log}
	go func() {
		ticker := time.NewTicker(cfg.ReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := scaler.Scale(ctx, nil); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("periodic reconcile failed", "error", err)
				}
			}
		}
	}()
	mux := http.NewServeMux()
	mux.Handle("/metrics", telemetry.Handler(scaler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	server := &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if e := server.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Error("metrics server", "error", e)
		}
	}()
	defer server.Shutdown(context.Background())
	l, err := listener.New(session, listener.Config{ScaleSetID: scaleSet.ID, MaxRunners: cfg.MaxRunners, Logger: log.WithGroup("listener")})
	if err != nil {
		return err
	}
	log.Info("controller ready", "scale_set_id", scaleSet.ID, "scale_set_name", scaleSet.Name, "max_runners", cfg.MaxRunners)
	if err = l.Run(ctx, scaler); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
