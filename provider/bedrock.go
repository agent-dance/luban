package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// Default model IDs used across constructor and env-config functions.
const (
	// DefaultBedrockModel is the default cross-region inference profile for Bedrock.
	DefaultBedrockModel = "anthropic.claude-sonnet-5"
	// DefaultVertexModel is the default model for Vertex AI.
	DefaultVertexModel = "claude-sonnet-5"
)

// BedrockConfig holds AWS-specific configuration for the Bedrock provider.
type BedrockConfig struct {
	// Region is the AWS region (e.g. "us-east-1"). Defaults to AWS_REGION / AWS_DEFAULT_REGION env.
	Region string

	// Model is the Bedrock model ID (e.g. "anthropic.claude-3-5-sonnet-20241022-v2:0").
	// Supports foundation models, cross-region inference profiles (e.g. "us.anthropic.claude-..."),
	// and ARN-format inference profiles.
	Model string

	// MaxTokens overrides the default max output tokens.
	MaxTokens int

	// Timeout in seconds for requests.
	Timeout int

	// StaticCredentials, if set, skips the default credential chain.
	// Populated from AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY + AWS_SESSION_TOKEN env vars.
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// BearerToken is set from AWS_BEARER_TOKEN_BEDROCK for API-key-style auth.
	BearerToken string

	// BaseURL overrides the Bedrock runtime endpoint (ANTHROPIC_BEDROCK_BASE_URL).
	// Must be empty, start with "https://", or start with "http://localhost" (for testing).
	BaseURL string
}

// BedrockConfigFromEnv reads Bedrock configuration from environment variables.
//
// Variables read:
//   - AWS_REGION / AWS_DEFAULT_REGION — AWS region (falls back to "us-east-1")
//   - AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN — static credentials
//   - AWS_BEARER_TOKEN_BEDROCK — bearer token for API-key-style auth
//   - ANTHROPIC_BEDROCK_BASE_URL — custom endpoint override
//   - BEDROCK_MODEL — model ID override
func BedrockConfigFromEnv() BedrockConfig {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	model := os.Getenv("BEDROCK_MODEL")
	if model == "" {
		model = DefaultBedrockModel
	}

	return BedrockConfig{
		Region:          region,
		Model:           model,
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		BearerToken:     os.Getenv("AWS_BEARER_TOKEN_BEDROCK"),
		BaseURL:         os.Getenv("ANTHROPIC_BEDROCK_BASE_URL"),
	}
}

// BedrockProvider wraps the Anthropic SDK with AWS Bedrock transport.
type BedrockProvider struct {
	client anthropic.Client
	model  string
}

// NewBedrock creates a Provider backed by Amazon Bedrock.
//
// Authentication priority (mirrors the Anthropic SDK behaviour):
//  1. AWS_BEARER_TOKEN_BEDROCK env / cfg.BearerToken — bearer token auth
//  2. cfg.AccessKeyID + cfg.SecretAccessKey — static SigV4 credentials
//  3. Default AWS credential chain (env → shared config → EC2 IMDS → …)
//
// ctx is passed to the AWS credential chain (e.g. for IMDS metadata fetches).
func NewBedrock(ctx context.Context, cfg BedrockConfig) (*BedrockProvider, error) {
	// B5: Validate BaseURL — must be empty, https://, or http://localhost (for tests).
	if err := validateBaseURL(cfg.BaseURL); err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderBedrockConfigInvalid, err)
	}

	awsCfg, err := buildAWSConfig(ctx, cfg)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderBedrockAWSConfigFailed, err)
	}

	var opts []option.RequestOption
	if cfg.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(time.Duration(cfg.Timeout)*time.Second))
	}
	// Bedrock uses AWS SigV4 (or bearer token) auth — not an Anthropic API key.
	// Pass an empty string to satisfy the SDK's key requirement without a sentinel value.
	opts = append(opts, option.WithAPIKey(""))
	opts = append(opts, bedrock.WithConfig(awsCfg))

	if cfg.BaseURL != "" {
		// Custom endpoint overrides the one injected by bedrock.WithConfig
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	model := cfg.Model
	if model == "" {
		model = DefaultBedrockModel
	}

	return &BedrockProvider{
		client: anthropic.NewClient(opts...),
		model:  model,
	}, nil
}

// validateBaseURL returns an error if url is non-empty and not a valid override URL.
// Allows https:// for production endpoints and http://localhost (exact host, with optional
// port or path) for local testing.  Rejects http://localhost.evil.com and similar SSRF
// bypass attempts that share the "http://localhost" prefix but are not localhost.
func validateBaseURL(url string) error {
	if url == "" {
		return nil
	}
	if strings.HasPrefix(url, "https://") {
		return nil
	}
	// Allow http://localhost with optional port and path, but nothing else.
	if url == "http://localhost" ||
		strings.HasPrefix(url, "http://localhost/") ||
		strings.HasPrefix(url, "http://localhost:") {
		return nil
	}
	return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderBedrockInvalidBaseURL, url))
}

// buildAWSConfig constructs an aws.Config from BedrockConfig.
// ctx is forwarded to the AWS SDK for operations like IMDS metadata fetches.
func buildAWSConfig(ctx context.Context, cfg BedrockConfig) (aws.Config, error) {
	// Bearer token takes precedence — build a minimal config with no credential chain.
	if cfg.BearerToken != "" {
		awsCfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Region),
			config.WithCredentialsProvider(aws.AnonymousCredentials{}),
		)
		if err != nil {
			return aws.Config{}, err
		}
		awsCfg.BearerAuthTokenProvider = bedrock.NewStaticBearerTokenProvider(cfg.BearerToken)
		return awsCfg, nil
	}

	// Static credentials provided explicitly
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		return config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Region),
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken),
			),
		)
	}

	// Fall through to default credential chain (shared config, IMDS, etc.)
	return config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
}

func (p *BedrockProvider) Name() string    { return "bedrock" }
func (p *BedrockProvider) ModelID() string { return p.model }

// Capabilities implements CapabilityProvider.
func (p *BedrockProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Thinking:     true,
		ToolUse:      true,
		CacheControl: true,
		SystemParts:  true,
		Vision:       true,
		MaxContext:   LookupMaxContext(p.model),
	}
}

// CreateStream uses the shared Anthropic stream implementation with our Bedrock client.
func (p *BedrockProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	if params.Model == "" {
		params.Model = p.model
	}
	if params.PromptCacheTTL == "" {
		params.PromptCacheTTL = anthropicPromptCacheTTL("bedrock", params.Model, "")
	}
	return createAnthropicStream(ctx, &p.client, params)
}
