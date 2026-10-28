package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	GitHubURL, GitHubToken, ScaleSetName, RunnerGroup string
	Labels                                            []string
	MinRunners, MaxRunners                            int
	ControllerID, Python, Adapter, RunnerTemplate     string
	RunnerTimeout, ReconcileInterval, OrphanGrace     time.Duration
	MetricsAddr                                       string
}

func FromEnv() (Config, error) {
	c := Config{
		GitHubURL: os.Getenv("GITHUB_CONFIG_URL"), GitHubToken: strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		ScaleSetName: value("SCALE_SET_NAME", "novita"), RunnerGroup: value("RUNNER_GROUP", "default"),
		ControllerID: value("CONTROLLER_ID", "novita-actions-controller"),
		Python:       value("PYTHON", "python3"), Adapter: value("NOVITA_ADAPTER", "/app/provisioner/novita/adapter.py"),
		RunnerTemplate: os.Getenv("NOVITA_RUNNER_TEMPLATE"), MetricsAddr: value("METRICS_ADDR", ":8080"),
	}
	c.Labels = split(value("RUNNER_LABELS", c.ScaleSetName))
	var err error
	if c.MinRunners, err = integer("MIN_RUNNERS", 0); err != nil {
		return c, err
	}
	if c.MaxRunners, err = integer("MAX_RUNNERS", 10); err != nil {
		return c, err
	}
	c.RunnerTimeout, err = duration("RUNNER_TIMEOUT", time.Hour)
	if err != nil {
		return c, err
	}
	c.ReconcileInterval, err = duration("RECONCILE_INTERVAL", 30*time.Second)
	if err != nil {
		return c, err
	}
	c.OrphanGrace, err = duration("ORPHAN_GRACE", 15*time.Minute)
	if err != nil {
		return c, err
	}
	if c.GitHubURL == "" || c.GitHubToken == "" || c.RunnerTemplate == "" {
		return c, fmt.Errorf("GITHUB_CONFIG_URL, GITHUB_TOKEN, and NOVITA_RUNNER_TEMPLATE are required")
	}
	if c.MinRunners < 0 || c.MaxRunners < c.MinRunners || c.MaxRunners == 0 {
		return c, fmt.Errorf("runner limits must satisfy 0 <= MIN_RUNNERS <= MAX_RUNNERS and MAX_RUNNERS > 0")
	}
	return c, nil
}
func value(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func split(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
func integer(k string, d int) (int, error) {
	v := value(k, strconv.Itoa(d))
	n, e := strconv.Atoi(v)
	if e != nil {
		return 0, fmt.Errorf("%s must be an integer", k)
	}
	return n, nil
}
func duration(k string, d time.Duration) (time.Duration, error) {
	v := value(k, d.String())
	x, e := time.ParseDuration(v)
	if e != nil || x <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", k)
	}
	return x, nil
}
