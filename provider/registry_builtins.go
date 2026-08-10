package provider

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/agent-dance/luban/brand"
)

// registerBuiltinProviders registers all built-in providers into the registry.
// This is called once by DefaultRegistry().
func registerBuiltinProviders(r *ProviderRegistry) {
	registerAnthropic(r)
	registerOpenAI(r)
	registerCompatibleAggregates(r)
	registerBedrock(r)
	registerVertex(r)
	registerOllama(r)
	registerDeepSeek(r)
	registerGemini(r)
	registerGroq(r)
	registerXAI(r)
	registerMistral(r)
	registerZhipu(r)
	registerMiniMax(r)
	registerKimi(r)
}

func registerCompatibleAggregates(r *ProviderRegistry) {
	definitions := []CompatibleProviderDefinition{
		{
			Name: "volcengine", DisplayName: "Volcengine Ark Coding Plan", Popularity: 88,
			BaseURLs: map[APIStyle]string{
				APIStyleOpenAI:    "https://ark.cn-beijing.volces.com/api/coding/v3",
				APIStyleAnthropic: "https://ark.cn-beijing.volces.com/api/coding",
			},
		},
		{
			Name: "alibaba-cloud", DisplayName: "Alibaba Cloud Coding Plan", Popularity: 87,
			BaseURLs: map[APIStyle]string{
				APIStyleOpenAI:    "https://coding.dashscope.aliyuncs.com/v1",
				APIStyleAnthropic: "https://coding.dashscope.aliyuncs.com/apps/anthropic",
			},
		},
		{
			Name: "tencent-cloud", DisplayName: "Tencent Cloud Coding Plan", Popularity: 86,
			BaseURLs: map[APIStyle]string{
				APIStyleOpenAI:    "https://api.lkeap.cloud.tencent.com/coding/v3",
				APIStyleAnthropic: "https://api.lkeap.cloud.tencent.com/coding/anthropic",
			},
		},
	}
	for _, definition := range definitions {
		r.RegisterCompatibleProvider(definition)
	}
}

func registerAnthropic(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "anthropic",
		DisplayName:    "Anthropic",
		EnvKey:         "ANTHROPIC_API_KEY",
		AuthMethods:    []string{"api_key", "oauth_pkce"},
		Popularity:     100,
		DefaultBaseURL: "https://api.anthropic.com",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := strings.TrimSpace(cfg.APIKey)
		authToken := strings.TrimSpace(cfg.AuthToken)
		oauthBacked := authToken != "" && apiKey == ""
		headers := mergeHeaders(loadEnvHeaders("ANTHROPIC_CUSTOM_HEADERS"), cfg.Headers)
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
		if authToken == "" {
			authToken = strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN"))
		}
		if apiKey == "" && authToken == "" {
			if hook := r.OAuthHookRef(); hook != nil {
				if token, err := hook.LoadAccessToken(context.Background()); err == nil && token != "" {
					authToken = token
					oauthBacked = true
				}
			}
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = os.Getenv("ANTHROPIC_MODEL")
		}
		if model == "" {
			model = CatalogDefaultModel("anthropic", "claude-sonnet-5")
		}
		if authToken != "" {
			// Bearer-token authentication must not also emit X-Api-Key from
			// environment or credential-store leftovers.
			apiKey = ""
		}
		if apiKey == "" && authToken == "" {
			return NewUnconfiguredProvider("anthropic", model, "ANTHROPIC_API_KEY", ""), nil
		}
		raw := NewAnthropic(Config{
			APIKey:    apiKey,
			AuthToken: authToken,
			BaseURL:   firstNonEmpty(cfg.BaseURL, os.Getenv("ANTHROPIC_BASE_URL")),
			Model:     model,
			Headers:   headers,
		})
		retryCfg := DefaultRetryConfig()
		if oauthBacked {
			if hook := r.OAuthHookRef(); hook != nil {
				retryCfg.OnAuthError = hook.OnAuthError
			}
		}
		return NewRetryProvider(raw, retryCfg), nil
	})
}

func registerOpenAI(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "openai",
		DisplayName:    "OpenAI",
		EnvKey:         "OPENAI_API_KEY",
		AuthMethods:    []string{"api_key", "oauth_pkce"},
		Popularity:     90,
		DefaultBaseURL: "https://api.openai.com/v1",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		authToken := strings.TrimSpace(cfg.AuthToken)
		apiKey := cfg.APIKey
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("OPENAI_MODEL", CatalogDefaultModel("openai", "gpt-5.6-sol"))
		}
		if authToken == "" && apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if authToken == "" && apiKey == "" {
			return NewUnconfiguredProvider("openai", model, "OPENAI_API_KEY", ""), nil
		}
		providerCfg := Config{
			ProviderName:              "openai",
			ResponsesSemantics:        ResponsesSemanticsOpenAIPublic,
			ResponsesWebSocket:        normalizeCapabilitySupport(cfg.ResponsesWebSocket),
			APIKey:                    apiKey,
			AuthToken:                 authToken,
			BaseURL:                   firstNonEmpty(cfg.BaseURL, os.Getenv("OPENAI_BASE_URL")),
			Model:                     model,
			Headers:                   cloneHeaders(cfg.Headers),
			DisablePromptCacheOptions: cfg.DisablePromptCacheOptions,
			CacheRoutingPreference:    cfg.CacheRoutingPreference,
		}
		// Transport location is not a capability signal. A content-blind proxy
		// for OpenAI public Responses must preserve the exact strict tool body;
		// genuinely incompatible gateways opt into the compatible provider or an
		// explicit DisableStrictTools setting.
		providerCfg.DisableStrictTools = cfg.DisableStrictTools

		// Keep API selection inside the OpenAI provider: explicit flags win,
		// first-party models use their cataloged format, and cataloged Responses
		// models negotiate that protocol on custom gateways with a chat fallback.
		apiFormat := normalizeOpenAIAPIFormat(firstNonEmpty(cfg.APIFormat, os.Getenv("OPENAI_API")))
		useResponses := resolveOpenAIResponsesMode(
			authToken,
			apiFormat,
			providerCfg.BaseURL,
			model,
		)
		if providerCfg.ResponsesWebSocket == CapabilitySupported {
			if (cfg.ResponsesSemantics != ResponsesSemanticsAuto && cfg.ResponsesSemantics != ResponsesSemanticsOpenAIPublic) ||
				authToken != "" || isOpenAIChatGPTCodexBaseURL(providerCfg.BaseURL) || !useResponses {
				return nil, responsesWebSocketProfileUnsupportedError()
			}
		}
		if useResponses {
			if authToken != "" {
				providerCfg.ResponsesSemantics = ResponsesSemanticsOpenAICodex
			}
			raw := NewResponses(providerCfg)
			retryCfg := DefaultRetryConfig()
			if authToken != "" {
				retryCfg.OnAuthError = openAIOAuthRefreshHandler(r, raw)
			}
			return NewRetryProvider(raw, retryCfg), nil
		}
		if shouldNegotiateOpenAIResponses(authToken, apiFormat, providerCfg.BaseURL, model) {
			return NewRetryProvider(newNegotiatingOpenAIProvider(providerCfg), DefaultRetryConfig()), nil
		}
		providerCfg.BaseURL = normalizeOpenAIChatBaseURL(providerCfg.BaseURL)
		raw := NewOpenAI(providerCfg)
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})

}

func registerBedrock(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:            "bedrock",
		DisplayName:     "Amazon Bedrock",
		EnvKey:          "", // Bedrock uses AWS credential chain, no single env key
		AuthMethods:     []string{"api_key", "aws_credentials"},
		Popularity:      70,
		RequiresContext: true,
	}, func(cfg Config, modelOverride string) (Provider, error) {
		bcfg := BedrockConfigFromEnv()
		if cfg.APIKey != "" {
			bcfg.BearerToken = cfg.APIKey
		}
		if cfg.BaseURL != "" {
			bcfg.BaseURL = cfg.BaseURL
		}
		if modelOverride != "" {
			bcfg.Model = modelOverride
		} else if cfg.Model != "" {
			bcfg.Model = cfg.Model
		}
		// TODO: thread a real context through factory callers.
		raw, err := NewBedrock(context.Background(), bcfg)
		if err != nil {
			return nil, err
		}
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerVertex(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:            "vertex",
		DisplayName:     "Google Vertex AI",
		EnvKey:          "", // Vertex uses GCP ADC
		AuthMethods:     []string{"api_key", "gcp_adc"},
		Popularity:      65,
		RequiresContext: true,
	}, func(cfg Config, modelOverride string) (Provider, error) {
		vcfg := VertexConfigFromEnv()
		if modelOverride != "" {
			vcfg.Model = modelOverride
		} else if cfg.Model != "" {
			vcfg.Model = cfg.Model
		}
		if cfg.APIKey != "" {
			raw, err := NewVertexCustomEndpoint(Config{
				APIKey:  cfg.APIKey,
				BaseURL: cfg.BaseURL,
				Model:   vcfg.Model,
				Headers: cloneHeaders(cfg.Headers),
			})
			if err != nil {
				return nil, err
			}
			return NewRetryProvider(raw, DefaultRetryConfig()), nil
		}
		// TODO: thread a real context through factory callers.
		raw, err := NewVertex(context.Background(), vcfg)
		if err != nil {
			return nil, err
		}
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerOllama(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "ollama",
		DisplayName:    "Ollama (Local)",
		EnvKey:         "", // Local server, no API key needed
		AuthMethods:    []string{"api_key"},
		Popularity:     60,
		DefaultBaseURL: "http://localhost:11434/v1",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("OLLAMA_MODEL", "llama3.1")
		}
		raw := NewOpenAI(Config{
			ProviderName:           "ollama",
			APIKey:                 cfg.APIKey,
			BaseURL:                firstNonEmpty(cfg.BaseURL, envOrDefault("OLLAMA_BASE_URL", "http://localhost:11434/v1")),
			Model:                  model,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		// Local inference: short retries, no need for long backoff.
		localRetry := RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   100 * time.Millisecond,
			MaxDelay:    1 * time.Second,
		}
		return NewRetryProvider(raw, localRetry), nil
	})
}

func registerDeepSeek(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "deepseek",
		DisplayName:    "DeepSeek",
		EnvKey:         "DEEPSEEK_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     50,
		DefaultBaseURL: brand.DeepSeekBaseURL,
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("DEEPSEEK_API_KEY")
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("DEEPSEEK_MODEL", brand.DeepSeekDefaultModel)
		}
		if apiKey == "" {
			return NewUnconfiguredProvider(brand.DeepSeekProvider, model, "DEEPSEEK_API_KEY", ""), nil
		}
		baseURL := firstNonEmpty(cfg.BaseURL, envOrDefault("DEEPSEEK_BASE_URL", brand.DeepSeekBaseURL))
		apiFormat := normalizeOpenAIAPIFormat(firstNonEmpty(cfg.APIFormat, os.Getenv("DEEPSEEK_API")))
		if apiFormat == "" && isFirstPartyDeepSeekBaseURL(baseURL) {
			apiFormat = modelCatalogOpenAIAPIFormat(r.catalog, brand.DeepSeekProvider, model)
		}
		if apiFormat == "" && isFirstPartyDeepSeekBaseURL(baseURL) {
			apiFormat = modelCatalogOpenAIAPIFormat(DefaultCatalog(), brand.DeepSeekProvider, model)
		}
		providerCfg := Config{
			ProviderName:              brand.DeepSeekProvider,
			APIFormat:                 apiFormat,
			ResponsesSemantics:        ResponsesSemanticsDeepSeek,
			APIKey:                    apiKey,
			BaseURL:                   baseURL,
			Model:                     model,
			MaxTokens:                 cfg.MaxTokens,
			Headers:                   cloneHeaders(cfg.Headers),
			DisableStrictTools:        cfg.DisableStrictTools,
			DisablePromptCacheOptions: cfg.DisablePromptCacheOptions,
			CacheRoutingPreference:    cfg.CacheRoutingPreference,
		}
		if apiFormat == "responses" {
			return NewRetryProvider(NewResponses(providerCfg), DefaultRetryConfig()), nil
		}
		raw := NewOpenAI(providerCfg)
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerGemini(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "gemini",
		DisplayName:    "Google Gemini",
		EnvKey:         "GEMINI_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     80,
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("GEMINI_MODEL", CatalogDefaultModel("gemini", "gemini-3.5-flash"))
		}
		if apiKey == "" {
			return NewUnconfiguredProvider("gemini", model, "GEMINI_API_KEY", ""), nil
		}
		raw := NewOpenAI(Config{
			ProviderName:           "gemini",
			APIKey:                 apiKey,
			BaseURL:                firstNonEmpty(cfg.BaseURL, envOrDefault("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai")),
			Model:                  model,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerGroq(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "groq",
		DisplayName:    "Groq",
		EnvKey:         "GROQ_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     55,
		DefaultBaseURL: "https://api.groq.com/openai/v1",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("GROQ_API_KEY")
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("GROQ_MODEL", CatalogDefaultModel("groq", "llama-3.3-70b-versatile"))
		}
		if apiKey == "" {
			return NewUnconfiguredProvider("groq", model, "GROQ_API_KEY", ""), nil
		}
		raw := NewOpenAI(Config{
			ProviderName:           "groq",
			APIKey:                 apiKey,
			BaseURL:                firstNonEmpty(cfg.BaseURL, envOrDefault("GROQ_BASE_URL", "https://api.groq.com/openai/v1")),
			Model:                  model,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerXAI(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "xai",
		DisplayName:    "xAI",
		EnvKey:         "XAI_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     54,
		DefaultBaseURL: "https://api.x.ai/v1",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("XAI_API_KEY")
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("XAI_MODEL", CatalogDefaultModel("xai", "grok-4.5"))
		}
		if apiKey == "" {
			return NewUnconfiguredProvider("xai", model, "XAI_API_KEY", ""), nil
		}
		raw := NewResponses(Config{
			ProviderName:           "xai",
			ResponsesSemantics:     ResponsesSemanticsCompatible,
			APIKey:                 apiKey,
			BaseURL:                normalizeOpenAIChatBaseURL(firstNonEmpty(cfg.BaseURL, envOrDefault("XAI_BASE_URL", "https://api.x.ai/v1"))),
			Model:                  model,
			DisableStrictTools:     true,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerMistral(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "mistral",
		DisplayName:    "Mistral AI",
		EnvKey:         "MISTRAL_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     45,
		DefaultBaseURL: "https://api.mistral.ai/v1",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("MISTRAL_API_KEY")
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("MISTRAL_MODEL", CatalogDefaultModel("mistral", "mistral-large-2512"))
		}
		if apiKey == "" {
			return NewUnconfiguredProvider("mistral", model, "MISTRAL_API_KEY", ""), nil
		}
		raw := NewOpenAI(Config{
			ProviderName:           "mistral",
			APIKey:                 apiKey,
			BaseURL:                firstNonEmpty(cfg.BaseURL, envOrDefault("MISTRAL_BASE_URL", "https://api.mistral.ai/v1")),
			Model:                  model,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerZhipu(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "zhipu",
		DisplayName:    "Zhipu AI",
		EnvKey:         "ZHIPU_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     58,
		DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = firstNonEmpty(os.Getenv("ZHIPU_API_KEY"), os.Getenv("BIGMODEL_API_KEY"))
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("ZHIPU_MODEL", CatalogDefaultModel("zhipu", "glm-5.2"))
		}
		if apiKey == "" {
			return NewUnconfiguredProvider("zhipu", model, "ZHIPU_API_KEY", ""), nil
		}
		raw := NewOpenAI(Config{
			ProviderName:           "zhipu",
			APIKey:                 apiKey,
			BaseURL:                firstNonEmpty(cfg.BaseURL, envOrDefault("ZHIPU_BASE_URL", "https://open.bigmodel.cn/api/paas/v4")),
			Model:                  model,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerMiniMax(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "minimax",
		DisplayName:    "MiniMax",
		EnvKey:         "MINIMAX_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     57,
		DefaultBaseURL: "https://api.minimaxi.com/v1",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("MINIMAX_API_KEY")
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("MINIMAX_MODEL", CatalogDefaultModel("minimax", "MiniMax-M3"))
		}
		if apiKey == "" {
			return NewUnconfiguredProvider("minimax", model, "MINIMAX_API_KEY", ""), nil
		}
		raw := NewOpenAI(Config{
			ProviderName:           "minimax",
			APIKey:                 apiKey,
			BaseURL:                firstNonEmpty(cfg.BaseURL, envOrDefault("MINIMAX_BASE_URL", "https://api.minimaxi.com/v1")),
			Model:                  model,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func registerKimi(r *ProviderRegistry) {
	r.Register(ProviderInfo{
		Name:           "kimi",
		DisplayName:    "Kimi (Moonshot AI)",
		EnvKey:         "MOONSHOT_API_KEY",
		AuthMethods:    []string{"api_key"},
		Popularity:     56,
		DefaultBaseURL: "https://api.moonshot.cn/v1",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = firstNonEmpty(os.Getenv("MOONSHOT_API_KEY"), os.Getenv("KIMI_API_KEY"))
		}
		model := modelOverride
		if model == "" && cfg.Model != "" {
			model = cfg.Model
		}
		if model == "" {
			model = envOrDefault("KIMI_MODEL", CatalogDefaultModel("kimi", "kimi-k3"))
		}
		if apiKey == "" {
			return NewUnconfiguredProvider("kimi", model, "MOONSHOT_API_KEY", ""), nil
		}
		raw := NewOpenAI(Config{
			ProviderName:           "kimi",
			APIKey:                 apiKey,
			BaseURL:                firstNonEmpty(cfg.BaseURL, envOrDefault("KIMI_BASE_URL", "https://api.moonshot.cn/v1")),
			Model:                  model,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

// firstNonEmpty returns the first non-empty string from its arguments.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
