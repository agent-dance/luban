package provider

import (
	"context"
	"testing"
)

func TestBedrockConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("BEDROCK_MODEL", "")
	t.Setenv("CLAUDE_MODEL", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "")
	t.Setenv("ANTHROPIC_BEDROCK_BASE_URL", "")

	cfg := BedrockConfigFromEnv()

	if cfg.Region != "us-east-1" {
		t.Errorf("expected default region us-east-1, got %q", cfg.Region)
	}
	if cfg.Model != "anthropic.claude-sonnet-5" {
		t.Errorf("expected default model anthropic.claude-sonnet-5, got %q", cfg.Model)
	}
	if cfg.AccessKeyID != "" {
		t.Errorf("expected empty AccessKeyID, got %q", cfg.AccessKeyID)
	}
}

func TestBedrockConfigFromEnv_ExplicitRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_DEFAULT_REGION", "")

	cfg := BedrockConfigFromEnv()
	if cfg.Region != "eu-west-1" {
		t.Errorf("expected eu-west-1, got %q", cfg.Region)
	}
}

func TestBedrockConfigFromEnv_DefaultRegionFallback(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "ap-southeast-1")

	cfg := BedrockConfigFromEnv()
	if cfg.Region != "ap-southeast-1" {
		t.Errorf("expected ap-southeast-1, got %q", cfg.Region)
	}
}

func TestBedrockConfigFromEnv_ModelFromEnv(t *testing.T) {
	t.Setenv("BEDROCK_MODEL", "us.anthropic.claude-opus-4-6-v1:0")
	t.Setenv("CLAUDE_MODEL", "")

	cfg := BedrockConfigFromEnv()
	if cfg.Model != "us.anthropic.claude-opus-4-6-v1:0" {
		t.Errorf("expected model from BEDROCK_MODEL, got %q", cfg.Model)
	}
}

func TestBedrockConfigFromEnv_ModelFromClaudeModelFallback(t *testing.T) {
	t.Setenv("BEDROCK_MODEL", "")
	t.Setenv("CLAUDE_MODEL", "anthropic.claude-3-5-sonnet-20241022-v2:0")

	cfg := BedrockConfigFromEnv()
	if cfg.Model != "anthropic.claude-3-5-sonnet-20241022-v2:0" {
		t.Errorf("expected model from CLAUDE_MODEL fallback, got %q", cfg.Model)
	}
}

func TestBedrockConfigFromEnv_StaticCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "mysessiontoken")

	cfg := BedrockConfigFromEnv()
	if cfg.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("unexpected AccessKeyID %q", cfg.AccessKeyID)
	}
	if cfg.SecretAccessKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("unexpected SecretAccessKey")
	}
	if cfg.SessionToken != "mysessiontoken" {
		t.Errorf("unexpected SessionToken %q", cfg.SessionToken)
	}
}

func TestBedrockConfigFromEnv_BearerToken(t *testing.T) {
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "mytoken123")

	cfg := BedrockConfigFromEnv()
	if cfg.BearerToken != "mytoken123" {
		t.Errorf("expected bearer token mytoken123, got %q", cfg.BearerToken)
	}
}

func TestBedrockConfigFromEnv_BaseURL(t *testing.T) {
	t.Setenv("ANTHROPIC_BEDROCK_BASE_URL", "https://custom.bedrock.example.com")

	cfg := BedrockConfigFromEnv()
	if cfg.BaseURL != "https://custom.bedrock.example.com" {
		t.Errorf("unexpected BaseURL %q", cfg.BaseURL)
	}
}

// TestNewBedrock_BearerToken verifies that NewBedrock succeeds when a bearer token
// is provided (no real AWS API call is made — only config construction is tested).
func TestNewBedrock_BearerToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping NewBedrock construction test in short mode")
	}
	cfg := BedrockConfig{
		Region:      "us-east-1",
		Model:       "anthropic.claude-sonnet-4-6",
		BearerToken: "dummy-bearer-token",
	}
	p, err := NewBedrock(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewBedrock with bearer token failed: %v", err)
	}
	if p.Name() != "bedrock" {
		t.Errorf("expected name bedrock, got %q", p.Name())
	}
	if p.ModelID() != "anthropic.claude-sonnet-4-6" {
		t.Errorf("unexpected ModelID %q", p.ModelID())
	}
}

// TestNewBedrock_StaticCredentials verifies construction with static AWS creds.
func TestNewBedrock_StaticCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping NewBedrock construction test in short mode")
	}
	cfg := BedrockConfig{
		Region:          "us-west-2",
		Model:           "anthropic.claude-3-5-sonnet-20241022-v2:0",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}
	p, err := NewBedrock(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewBedrock with static creds failed: %v", err)
	}
	if p.ModelID() != cfg.Model {
		t.Errorf("unexpected ModelID %q", p.ModelID())
	}
}

func TestBedrockProvider_Capabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	cfg := BedrockConfig{
		Region:      "us-east-1",
		Model:       "anthropic.claude-sonnet-4-6",
		BearerToken: "dummy",
	}
	p, err := NewBedrock(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewBedrock failed: %v", err)
	}
	caps := p.Capabilities()
	if !caps.ToolUse {
		t.Error("expected ToolUse=true")
	}
	if !caps.Thinking {
		t.Error("expected Thinking=true")
	}
	if caps.MaxContext != 1000000 {
		t.Errorf("MaxContext = %d, want 1000000", caps.MaxContext)
	}
}

// TestNewFromEnv_Bedrock verifies that CLAUDE_CODE_USE_BEDROCK=1 routes to bedrock.
// We set a bearer token so no real AWS call is needed.
func TestNewFromEnv_Bedrock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping env routing test in short mode")
	}
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("PROVIDER", "")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "dummy-token")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("BEDROCK_MODEL", "anthropic.claude-sonnet-4-6")

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv with CLAUDE_CODE_USE_BEDROCK=1 failed: %v", err)
	}
	// Unwrap RetryProvider to check underlying provider name.
	rp, ok := p.(*RetryProvider)
	if !ok {
		t.Fatalf("expected *RetryProvider, got %T", p)
	}
	if rp.inner.Name() != "bedrock" {
		t.Errorf("expected inner provider bedrock, got %q", rp.inner.Name())
	}
}

// TestNewBedrock_BaseURLValidation verifies that invalid BaseURL values are rejected
// and valid ones (https:// and http://localhost) are accepted.
func TestNewBedrock_BaseURLValidation(t *testing.T) {
	base := BedrockConfig{
		Region:      "us-east-1",
		BearerToken: "dummy",
	}

	// Invalid schemes should be rejected.
	invalid := []string{
		"http://example.com",
		"ftp://example.com",
		"//example.com",
		"example.com",
		// C2: SSRF bypass — shares "http://localhost" prefix but is not localhost.
		"http://localhost.evil.com",
		"http://localhost.evil.com:8080",
		"http://localhostevil.com",
	}
	for _, u := range invalid {
		cfg := base
		cfg.BaseURL = u
		_, err := NewBedrock(context.Background(), cfg)
		if err == nil {
			t.Errorf("expected error for BaseURL %q, got nil", u)
		}
	}

	// Valid values should be accepted.
	valid := []string{
		"",
		"https://custom.bedrock.example.com",
		"http://localhost:8080",
		"http://localhost",
		"http://localhost/path",
	}
	for _, u := range valid {
		cfg := base
		cfg.BaseURL = u
		_, err := NewBedrock(context.Background(), cfg)
		if err != nil {
			t.Errorf("unexpected error for BaseURL %q: %v", u, err)
		}
	}
}
