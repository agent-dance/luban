package provider

import (
	"context"
	"testing"
)

// --- VertexConfigFromEnv tests ---

func TestVertexConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "")
	t.Setenv("GOOGLE_CLOUD_REGION", "")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("ANTHROPIC_VERTEX_REGION", "")
	t.Setenv("VERTEX_MODEL", "")
	t.Setenv("CLAUDE_MODEL", "")

	cfg := VertexConfigFromEnv()

	if cfg.Region != "us-east5" {
		t.Errorf("expected default region us-east5, got %q", cfg.Region)
	}
	if cfg.Model != "claude-sonnet-5" {
		t.Errorf("expected default model claude-sonnet-5, got %q", cfg.Model)
	}
	if cfg.ProjectID != "" {
		t.Errorf("expected empty ProjectID when no env set, got %q", cfg.ProjectID)
	}
}

func TestVertexConfigFromEnv_ProjectFromGoogleCloudProject(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "my-gcp-project")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "")

	cfg := VertexConfigFromEnv()
	if cfg.ProjectID != "my-gcp-project" {
		t.Errorf("expected project from GOOGLE_CLOUD_PROJECT, got %q", cfg.ProjectID)
	}
}

func TestVertexConfigFromEnv_ProjectFromAnthropicVertexProjectID(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "my-anthropic-project")

	cfg := VertexConfigFromEnv()
	if cfg.ProjectID != "my-anthropic-project" {
		t.Errorf("expected project from ANTHROPIC_VERTEX_PROJECT_ID, got %q", cfg.ProjectID)
	}
}

func TestVertexConfigFromEnv_ProjectPriority(t *testing.T) {
	// GOOGLE_CLOUD_PROJECT takes precedence over ANTHROPIC_VERTEX_PROJECT_ID.
	t.Setenv("GOOGLE_CLOUD_PROJECT", "gcp-project")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "anthropic-project")

	cfg := VertexConfigFromEnv()
	if cfg.ProjectID != "gcp-project" {
		t.Errorf("expected GOOGLE_CLOUD_PROJECT to take precedence, got %q", cfg.ProjectID)
	}
}

func TestVertexConfigFromEnv_RegionFromGoogleCloudRegion(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_REGION", "us-central1")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("ANTHROPIC_VERTEX_REGION", "")

	cfg := VertexConfigFromEnv()
	if cfg.Region != "us-central1" {
		t.Errorf("expected region from GOOGLE_CLOUD_REGION, got %q", cfg.Region)
	}
}

func TestVertexConfigFromEnv_RegionFromCloudMLRegion(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_REGION", "")
	t.Setenv("CLOUD_ML_REGION", "europe-west4")
	t.Setenv("ANTHROPIC_VERTEX_REGION", "")

	cfg := VertexConfigFromEnv()
	if cfg.Region != "europe-west4" {
		t.Errorf("expected region from CLOUD_ML_REGION, got %q", cfg.Region)
	}
}

func TestVertexConfigFromEnv_RegionFromAnthropicVertexRegion(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_REGION", "")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("ANTHROPIC_VERTEX_REGION", "asia-southeast1")

	cfg := VertexConfigFromEnv()
	if cfg.Region != "asia-southeast1" {
		t.Errorf("expected region from ANTHROPIC_VERTEX_REGION, got %q", cfg.Region)
	}
}

func TestVertexConfigFromEnv_RegionPriority(t *testing.T) {
	// GOOGLE_CLOUD_REGION > CLOUD_ML_REGION > ANTHROPIC_VERTEX_REGION
	t.Setenv("GOOGLE_CLOUD_REGION", "us-central1")
	t.Setenv("CLOUD_ML_REGION", "europe-west4")
	t.Setenv("ANTHROPIC_VERTEX_REGION", "asia-southeast1")

	cfg := VertexConfigFromEnv()
	if cfg.Region != "us-central1" {
		t.Errorf("expected GOOGLE_CLOUD_REGION to win, got %q", cfg.Region)
	}
}

func TestVertexConfigFromEnv_ModelFromVertexModel(t *testing.T) {
	t.Setenv("VERTEX_MODEL", "claude-3-5-sonnet-v2@20241022")
	t.Setenv("CLAUDE_MODEL", "")

	cfg := VertexConfigFromEnv()
	if cfg.Model != "claude-3-5-sonnet-v2@20241022" {
		t.Errorf("expected model from VERTEX_MODEL, got %q", cfg.Model)
	}
}

func TestVertexConfigFromEnv_ModelFromClaudeModelFallback(t *testing.T) {
	t.Setenv("VERTEX_MODEL", "")
	t.Setenv("CLAUDE_MODEL", "claude-3-5-haiku-20241022")

	cfg := VertexConfigFromEnv()
	if cfg.Model != "claude-3-5-haiku-20241022" {
		t.Errorf("expected model from CLAUDE_MODEL fallback, got %q", cfg.Model)
	}
}

// --- NewVertex error cases ---

// TestNewVertex_MissingProject verifies that NewVertex returns an error when
// no project ID is provided. No real GCP credential loading occurs because
// the function checks ProjectID before calling vertex.WithGoogleAuth.
func TestNewVertex_MissingProject(t *testing.T) {
	cfg := VertexConfig{
		Region:    "us-east5",
		ProjectID: "", // intentionally empty
		Model:     "claude-sonnet-4-6",
	}
	_, err := NewVertex(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for missing ProjectID, got nil")
	}
}

// --- Provider-routing tests (no real credentials) ---

// TestNewFromEnv_VertexFlagMissingProject verifies that CLAUDE_CODE_USE_VERTEX=1
// routes to the vertex provider, which returns an error when no project is set.
func TestNewFromEnv_VertexFlagMissingProject(t *testing.T) {
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")
	t.Setenv("PROVIDER", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "")

	_, err := NewFromEnv()
	if err == nil {
		t.Error("expected error when project ID is not set, got nil")
	}
}

// TestNewFromEnv_VertexProviderMissingProject verifies that PROVIDER=vertex
// also routes to the vertex provider and surfaces missing-project errors.
func TestNewFromEnv_VertexProviderMissingProject(t *testing.T) {
	t.Setenv("PROVIDER", "vertex")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "")

	_, err := NewFromEnv()
	if err == nil {
		t.Error("expected error for vertex provider without project ID, got nil")
	}
}

// TestNewFromEnv_VertexFlagNotTriggeredWhenProviderSet ensures that
// CLAUDE_CODE_USE_VERTEX=1 does not override an explicit PROVIDER setting.
func TestNewFromEnv_VertexFlagNotTriggeredWhenProviderSet(t *testing.T) {
	// PROVIDER=ollama should win over CLAUDE_CODE_USE_VERTEX=1
	t.Setenv("PROVIDER", "ollama")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Ollama uses the OpenAI-compatible implementation under the hood.
	rp, ok := p.(*RetryProvider)
	if !ok {
		t.Fatalf("expected *RetryProvider, got %T", p)
	}
	if rp.inner.Name() != "ollama" {
		t.Errorf("expected ollama, got %q", rp.inner.Name())
	}
}
