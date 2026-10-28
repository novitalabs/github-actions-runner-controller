package config

import "testing"

func TestFromEnvReadsGitHubTokenFromEnvironment(t *testing.T) {
	t.Setenv("GITHUB_CONFIG_URL", "https://github.com/example/repository")
	t.Setenv("GITHUB_TOKEN", "  token-from-env  ")
	t.Setenv("NOVITA_RUNNER_TEMPLATE", "github-actions-runner")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.GitHubToken != "token-from-env" {
		t.Fatalf("GitHubToken = %q, want token-from-env", cfg.GitHubToken)
	}
}
