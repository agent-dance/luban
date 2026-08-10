package provider

import (
	"context"
	"fmt"
	"os"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// VertexConfig holds Google Cloud configuration for the Vertex AI provider.
type VertexConfig struct {
	// ProjectID is the GCP project ID (GOOGLE_CLOUD_PROJECT / ANTHROPIC_VERTEX_PROJECT_ID).
	ProjectID string

	// Region is the GCP region (e.g. "us-east5"). Defaults to GOOGLE_CLOUD_REGION /
	// CLOUD_ML_REGION / ANTHROPIC_VERTEX_REGION env vars, falls back to "us-east5".
	Region string

	// Model is the Vertex model ID (e.g. "claude-sonnet-4-20250514").
	// Vertex uses the same short model names as the direct API (no "anthropic." prefix).
	Model string

	// MaxTokens overrides the default max output tokens.
	MaxTokens int

	// Timeout in seconds for requests.
	Timeout int
}

// VertexConfigFromEnv reads Vertex AI configuration from environment variables.
//
// Variables read:
//   - GOOGLE_CLOUD_PROJECT / ANTHROPIC_VERTEX_PROJECT_ID — GCP project ID
//   - GOOGLE_CLOUD_REGION / CLOUD_ML_REGION / ANTHROPIC_VERTEX_REGION — GCP region
//   - VERTEX_MODEL — model ID override
//   - GOOGLE_APPLICATION_CREDENTIALS — path to service account JSON (used by ADC automatically)
func VertexConfigFromEnv() VertexConfig {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	}

	region := os.Getenv("GOOGLE_CLOUD_REGION")
	if region == "" {
		region = os.Getenv("CLOUD_ML_REGION")
	}
	if region == "" {
		region = os.Getenv("ANTHROPIC_VERTEX_REGION")
	}
	if region == "" {
		region = "us-east5"
	}

	model := os.Getenv("VERTEX_MODEL")
	if model == "" {
		model = DefaultVertexModel
	}

	return VertexConfig{
		ProjectID: projectID,
		Region:    region,
		Model:     model,
	}
}

// VertexProvider wraps the Anthropic SDK with Google Vertex AI transport.
type VertexProvider struct {
	client anthropic.Client
	model  string
}

// NewVertex creates a Provider backed by Google Cloud Vertex AI.
//
// Authentication uses Application Default Credentials (ADC):
//  1. GOOGLE_APPLICATION_CREDENTIALS env → service account JSON
//  2. gcloud application-default credentials
//  3. Workload Identity (GKE / Cloud Run)
//
// ctx is forwarded to vertex.WithGoogleAuth for credential initialization.
//
// Note: ADC errors are deferred — auth failures surface on the first API call,
// not at construction time. Use a health-check or test call to validate credentials
// eagerly if needed.
func NewVertex(ctx context.Context, cfg VertexConfig) (*VertexProvider, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderVertexProjectRequired))
	}

	// RetryProvider/AttemptController owns the request budget; disable the
	// Anthropic SDK's independent two-retry loop.
	opts := []option.RequestOption{option.WithMaxRetries(0)}
	if cfg.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(time.Duration(cfg.Timeout)*time.Second))
	}
	// Vertex AI uses OAuth2 Application Default Credentials — not an Anthropic API key.
	// Pass an empty string to satisfy the SDK's key requirement without a sentinel value.
	opts = append(opts, option.WithAPIKey(""))
	opts = append(opts, vertex.WithGoogleAuth(ctx, cfg.Region, cfg.ProjectID))

	model := cfg.Model
	if model == "" {
		model = DefaultVertexModel
	}

	return &VertexProvider{
		client: anthropic.NewClient(opts...),
		model:  model,
	}, nil
}

// NewVertexCustomEndpoint creates a Vertex-named provider backed by an
// Anthropic Messages-compatible endpoint using an API key. The normal Vertex
// path continues to use Google ADC.
func NewVertexCustomEndpoint(cfg Config) (*VertexProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderVertexAPIKeyRequired))
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderVertexBaseURLRequired))
	}
	if err := validateBaseURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderVertexEndpointInvalid), err)
	}
	direct := NewAnthropic(cfg)
	return &VertexProvider{client: direct.client, model: direct.model}, nil
}

func (p *VertexProvider) Name() string    { return "vertex" }
func (p *VertexProvider) ModelID() string { return p.model }

// Capabilities implements CapabilityProvider.
func (p *VertexProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Thinking:     true,
		ToolUse:      true,
		CacheControl: true,
		SystemParts:  true,
		Vision:       true,
		MaxContext:   LookupMaxContext(p.model),
	}
}

// CreateStream uses the shared Anthropic stream implementation with our Vertex client.
func (p *VertexProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	if err := ValidateParams(p, params); err != nil {
		return nil, err
	}
	if params.Model == "" {
		params.Model = p.model
	}
	if params.PromptCacheTTL == "" {
		params.PromptCacheTTL = anthropicPromptCacheTTL("vertex", params.Model, "")
	}
	return createAnthropicStream(ctx, &p.client, params)
}
