package provider

import (
	"os"
	"path/filepath"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// ConnectionState describes the user's effective ability to use a provider.
type ConnectionState string

const (
	ConnectionStateConnected     ConnectionState = "connected"
	ConnectionStateNotConfigured ConnectionState = "not_configured"
	ConnectionStateLocal         ConnectionState = "local_unverified"
	ConnectionStateUnknown       ConnectionState = "unknown"
)

// ConnectionKind identifies the authentication or backend class.
type ConnectionKind string

const (
	ConnectionKindAPIKey         ConnectionKind = "api_key"
	ConnectionKindOAuth          ConnectionKind = "oauth"
	ConnectionKindEnv            ConnectionKind = "env"
	ConnectionKindAWSCredentials ConnectionKind = "aws_credentials"
	ConnectionKindGCPADC         ConnectionKind = "gcp_adc"
	ConnectionKindLocalService   ConnectionKind = "local_service"
	ConnectionKindNone           ConnectionKind = "none"
)

// ConnectionDetail is the canonical provider connection status consumed by
// commands and TUI code. CanSelectModels is intentionally separate from State:
// local providers may be selectable without being verified as connected.
type ConnectionDetail struct {
	Provider        string
	DisplayName     string
	State           ConnectionState
	Kind            ConnectionKind
	Source          string
	Detail          string
	DetailKey       i18n.Key
	DetailArgs      []any
	CanSelectModels bool
	CanConnect      bool
	SetupHint       string
	SetupHintKey    i18n.Key
	SetupHintArgs   []any
}

// Localized returns a display-ready copy while retaining Detail and SetupHint
// as English compatibility fields for callers that have not migrated yet.
func (d ConnectionDetail) Localized(lang i18n.Language) ConnectionDetail {
	if d.DetailKey != "" {
		d.Detail = i18n.Format(lang, d.DetailKey, d.DetailArgs...)
	}
	if d.SetupHintKey != "" {
		d.SetupHint = i18n.Format(lang, d.SetupHintKey, d.SetupHintArgs...)
	}
	return d
}

func (d *ConnectionDetail) setDetail(key i18n.Key, args ...any) {
	d.DetailKey = key
	d.DetailArgs = append([]any(nil), args...)
	d.Detail = i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func (d *ConnectionDetail) setSetupHint(key i18n.Key, args ...any) {
	d.SetupHintKey = key
	d.SetupHintArgs = append([]any(nil), args...)
	d.SetupHint = i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

// ConnectionState returns a typed connection status for a registered provider.
func (r *ProviderRegistry) ConnectionState(name string) ConnectionDetail {
	info, ok := r.Get(name)
	if !ok {
		detail := ConnectionDetail{
			Provider:    name,
			DisplayName: name,
			State:       ConnectionStateUnknown,
			Kind:        ConnectionKindNone,
		}
		detail.setDetail(i18n.KeyProviderConnectionUnknown)
		detail.setSetupHint(i18n.KeyProviderConnectionChooseRegistered)
		return detail
	}
	return r.connectionStateForInfo(info)
}

func (r *ProviderRegistry) connectionStateForInfo(info ProviderInfo) ConnectionDetail {
	displayName := providerDisplayName(info)
	base := ConnectionDetail{
		Provider:    info.Name,
		DisplayName: displayName,
		State:       ConnectionStateNotConfigured,
		Kind:        primaryConnectionKind(info),
		CanConnect:  providerHasInteractiveSetup(info) || providerHasExternalSetup(info),
	}
	base.setDetail(i18n.KeyProviderConnectionNotConnected)
	setSetupHintForProvider(&base, info)

	if entry, ok := r.credentialEntry(info.Name); ok {
		if detail, connected := credentialConnectionDetail(info, entry); connected {
			return detail
		}
	}

	if info.EnvKey != "" {
		if os.Getenv(info.EnvKey) != "" {
			base.State = ConnectionStateConnected
			base.Kind = ConnectionKindEnv
			base.Source = info.EnvKey
			base.setDetail(i18n.KeyProviderConnectionConnectedEnv, info.EnvKey)
			base.CanSelectModels = true
			base.CanConnect = false
			return base
		}
		base.setDetail(i18n.KeyProviderConnectionEnvNotSet, info.EnvKey)
	}

	if info.Name == "anthropic" && os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" {
		base.State = ConnectionStateConnected
		base.Kind = ConnectionKindEnv
		base.Source = "ANTHROPIC_AUTH_TOKEN"
		base.setDetail(i18n.KeyProviderConnectionEnvSet, "ANTHROPIC_AUTH_TOKEN")
		base.CanSelectModels = true
		base.CanConnect = false
		return base
	}
	if info.Name == "anthropic" && os.Getenv("OAUTH_ACCESS_TOKEN") != "" {
		base.State = ConnectionStateConnected
		base.Kind = ConnectionKindEnv
		base.Source = "OAUTH_ACCESS_TOKEN"
		base.setDetail(i18n.KeyProviderConnectionEnvSet, "OAUTH_ACCESS_TOKEN")
		base.CanSelectModels = true
		base.CanConnect = false
		return base
	}

	switch info.Name {
	case "bedrock":
		return bedrockConnectionDetail(base)
	case "vertex":
		return vertexConnectionDetail(base)
	case "ollama":
		base.State = ConnectionStateLocal
		base.Kind = ConnectionKindLocalService
		base.Source = "local"
		base.setDetail(i18n.KeyProviderConnectionLocalUnchecked)
		base.CanSelectModels = true
		base.CanConnect = false
		base.setSetupHint(i18n.KeyProviderSetupOllama)
		return base
	}

	return base
}

func (r *ProviderRegistry) credentialEntry(providerName string) (CredentialEntry, bool) {
	cs := r.CredentialStoreRef()
	if cs == nil {
		return CredentialEntry{}, false
	}
	for _, lookupName := range CredentialLookupNames(providerName) {
		if entry, ok := cs.Get(lookupName); ok {
			return entry, true
		}
	}
	return CredentialEntry{}, false
}

func credentialConnectionDetail(info ProviderInfo, entry CredentialEntry) (ConnectionDetail, bool) {
	displayName := providerDisplayName(info)
	detail := ConnectionDetail{
		Provider:    info.Name,
		DisplayName: displayName,
		State:       ConnectionStateConnected,
		Source:      "credential_store",
		CanConnect:  false,
	}

	switch entry.AuthMethod {
	case "api_key":
		if entry.APIKey == "" {
			return ConnectionDetail{}, false
		}
		detail.Kind = ConnectionKindAPIKey
		detail.setDetail(i18n.KeyProviderConnectionCredentialAPIKey)
	case "env":
		if entry.APIKey == "" {
			return ConnectionDetail{}, false
		}
		detail.Kind = ConnectionKindEnv
		detail.setDetail(i18n.KeyProviderConnectionImportedEnv)
	case "oauth":
		if entry.AccessToken == "" && entry.RefreshToken == "" {
			return ConnectionDetail{}, false
		}
		if entry.AccessToken != "" && (entry.ExpiresAt.IsZero() || time.Now().Before(entry.ExpiresAt)) {
			detail.Kind = ConnectionKindOAuth
			detail.setDetail(i18n.KeyProviderConnectionOAuth)
		} else if entry.RefreshToken != "" {
			detail.Kind = ConnectionKindOAuth
			detail.setDetail(i18n.KeyProviderConnectionOAuthRefresh)
		} else {
			return ConnectionDetail{}, false
		}
		if CanonicalProviderName(info.Name) == "openai" {
			if entry.AccessToken != "" && (entry.ExpiresAt.IsZero() || time.Now().Before(entry.ExpiresAt)) {
				detail.setDetail(i18n.KeyProviderConnectionChatGPTOAuth)
			} else {
				detail.setDetail(i18n.KeyProviderConnectionChatGPTOAuthRefresh)
			}
		}
	default:
		return ConnectionDetail{}, false
	}

	detail.CanSelectModels = true
	return detail, true
}

func bedrockConnectionDetail(base ConnectionDetail) ConnectionDetail {
	base.Kind = ConnectionKindAWSCredentials
	switch {
	case os.Getenv("AWS_BEARER_TOKEN_BEDROCK") != "":
		base.State = ConnectionStateConnected
		base.Source = "AWS_BEARER_TOKEN_BEDROCK"
		base.setDetail(i18n.KeyProviderConnectionAWSBearer)
		base.CanSelectModels = true
		base.CanConnect = false
	case os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "":
		base.State = ConnectionStateConnected
		base.Source = "AWS_ACCESS_KEY_ID"
		base.setDetail(i18n.KeyProviderConnectionAWSAccessKey)
		base.CanSelectModels = true
		base.CanConnect = false
	case os.Getenv("AWS_PROFILE") != "":
		base.State = ConnectionStateConnected
		base.Source = "AWS_PROFILE"
		base.setDetail(i18n.KeyProviderConnectionAWSProfile)
		base.CanSelectModels = true
		base.CanConnect = false
	default:
		base.setDetail(i18n.KeyProviderConnectionAWSMissing)
		base.setSetupHint(i18n.KeyProviderSetupBedrock)
	}
	return base
}

func vertexConnectionDetail(base ConnectionDetail) ConnectionDetail {
	base.Kind = ConnectionKindGCPADC
	projectSource := ""
	if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" {
		projectSource = "GOOGLE_CLOUD_PROJECT"
	} else if os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID") != "" {
		projectSource = "ANTHROPIC_VERTEX_PROJECT_ID"
	}

	switch {
	case os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" && projectSource != "":
		base.State = ConnectionStateConnected
		base.Source = "GOOGLE_APPLICATION_CREDENTIALS"
		base.setDetail(i18n.KeyProviderConnectionGCPApplication)
		base.CanSelectModels = true
		base.CanConnect = false
	case defaultADCFileExists() && projectSource != "":
		base.State = ConnectionStateConnected
		base.Source = projectSource
		base.setDetail(i18n.KeyProviderConnectionGCPDefault)
		base.CanSelectModels = true
		base.CanConnect = false
	case os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" && projectSource == "":
		base.setDetail(i18n.KeyProviderConnectionGCPProjectMissing)
	case projectSource != "":
		base.setDetail(i18n.KeyProviderConnectionGCPADCMissing)
	default:
		base.setDetail(i18n.KeyProviderConnectionGCPMissing)
	}
	base.setSetupHint(i18n.KeyProviderSetupVertex)
	return base
}

func defaultADCFileExists() bool {
	if appData := os.Getenv("APPDATA"); appData != "" {
		if fileExists(filepath.Join(appData, "gcloud", "application_default_credentials.json")) {
			return true
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	return fileExists(filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func providerDisplayName(info ProviderInfo) string {
	if info.DisplayName != "" {
		return info.DisplayName
	}
	return info.Name
}

func primaryConnectionKind(info ProviderInfo) ConnectionKind {
	if info.Name == "ollama" {
		return ConnectionKindLocalService
	}
	for _, method := range info.AuthMethods {
		switch method {
		case "api_key":
			return ConnectionKindAPIKey
		case "oauth_pkce", "device_code":
			return ConnectionKindOAuth
		case "aws_credentials":
			return ConnectionKindAWSCredentials
		case "gcp_adc":
			return ConnectionKindGCPADC
		}
	}
	if info.EnvKey != "" {
		return ConnectionKindAPIKey
	}
	return ConnectionKindNone
}

func providerHasInteractiveSetup(info ProviderInfo) bool {
	for _, method := range info.AuthMethods {
		switch method {
		case "api_key", "oauth_pkce", "device_code":
			return true
		}
	}
	return info.EnvKey != ""
}

func providerHasExternalSetup(info ProviderInfo) bool {
	for _, method := range info.AuthMethods {
		switch method {
		case "aws_credentials", "gcp_adc":
			return true
		}
	}
	return info.Name == "ollama"
}

func setSetupHintForProvider(detail *ConnectionDetail, info ProviderInfo) {
	switch info.Name {
	case "bedrock":
		detail.setSetupHint(i18n.KeyProviderSetupBedrock)
		return
	case "vertex":
		detail.setSetupHint(i18n.KeyProviderSetupVertex)
		return
	case "ollama":
		detail.setSetupHint(i18n.KeyProviderSetupOllama)
		return
	}
	if info.EnvKey != "" {
		detail.setSetupHint(i18n.KeyProviderSetupEnvOrConnect, info.EnvKey, info.Name)
		return
	}
	detail.setSetupHint(i18n.KeyProviderSetupGeneric)
}
