package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// DeviceAuthConfig holds the parameters for an RFC 8628 Device Authorization Grant.
type DeviceAuthConfig struct {
	// ClientID is the OAuth client identifier.
	ClientID string

	// DeviceAuthURL is the device authorization endpoint.
	DeviceAuthURL string

	// TokenURL is the token endpoint used for polling.
	TokenURL string

	// Scopes are the requested OAuth scopes.
	Scopes []string

	// PollInterval overrides the server-suggested polling interval.
	// If zero, the server-provided interval is used (default 5s).
	PollInterval time.Duration
}

// DeviceCodeResponse is the initial response from the device authorization endpoint.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"` // poll interval in seconds
}

// StartDeviceAuthFlow runs a complete RFC 8628 device authorization flow:
//  1. Requests a device code from the authorization server.
//  2. Returns the device code response so the caller can display the user code.
//  3. Polls the token endpoint until the user authorizes or the flow times out.
//
// The onDeviceCode callback is invoked with the DeviceCodeResponse so the
// caller can display the verification URI and user code to the user.
// If onDeviceCode is nil, the response is silently consumed.
func StartDeviceAuthFlow(
	ctx context.Context,
	cfg DeviceAuthConfig,
	onDeviceCode func(DeviceCodeResponse),
) (*TokenResponse, error) {
	// Step 1: Request device code.
	dcr, err := requestDeviceCode(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Notify caller so they can display the user code.
	if onDeviceCode != nil {
		onDeviceCode(*dcr)
	}

	// Step 2: Poll for token.
	interval := time.Duration(dcr.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second // default per RFC 8628
	}
	if cfg.PollInterval > 0 {
		interval = cfg.PollInterval
	}

	expiry := time.Duration(dcr.ExpiresIn) * time.Second
	if expiry <= 0 {
		expiry = 10 * time.Minute // sensible default
	}

	deadline := time.Now().Add(expiry)

	for {
		select {
		case <-ctx.Done():
			return nil, i18n.WrapError(i18n.KeyAuthDeviceCancelled, ctx.Err())
		case <-time.After(interval):
		}

		if time.Now().After(deadline) {
			return nil, i18n.NewError(i18n.KeyAuthDeviceCodeExpiredAfter, expiry)
		}

		tr, err := pollDeviceToken(ctx, cfg, dcr.DeviceCode)
		if err != nil {
			var pollErr *devicePollError
			if isPollError(err, &pollErr) {
				switch pollErr.ErrorCode {
				case "authorization_pending":
					continue
				case "slow_down":
					interval += 5 * time.Second // RFC 8628 §3.5
					continue
				case "expired_token":
					return nil, i18n.NewError(i18n.KeyAuthDeviceCodeExpired)
				case "access_denied":
					return nil, i18n.NewError(i18n.KeyAuthDeviceAuthorizationDenied)
				}
			}
			return nil, err
		}

		return tr, nil
	}
}

// devicePollError represents a structured error from the token endpoint during polling.
type devicePollError struct {
	ErrorCode   string `json:"error"`
	Description string `json:"error_description"`
}

func (e *devicePollError) Error() string {
	if e.Description != "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuthDeviceRemoteErrorDetail, e.ErrorCode, e.Description)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuthDeviceRemoteError, e.ErrorCode)
}

// isPollError checks if an error is a devicePollError and extracts it.
func isPollError(err error, target **devicePollError) bool {
	if e, ok := err.(*devicePollError); ok {
		*target = e
		return true
	}
	return false
}

// requestDeviceCode sends the initial device authorization request.
func requestDeviceCode(ctx context.Context, cfg DeviceAuthConfig) (*DeviceCodeResponse, error) {
	vals := url.Values{}
	vals.Set("client_id", cfg.ClientID)
	if len(cfg.Scopes) > 0 {
		vals.Set("scope", strings.Join(cfg.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.DeviceAuthURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthDeviceBuildCodeRequest, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := authHTTPClient.Do(req)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthDeviceCodeRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, i18n.NewError(i18n.KeyAuthDeviceCodeEndpointRejected, resp.StatusCode, string(body))
	}

	var dcr DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthDeviceDecodeCodeResponse, err)
	}
	return &dcr, nil
}

// pollDeviceToken sends a single token request using the device code.
// Returns a *TokenResponse on success, or a *devicePollError for expected
// polling states (authorization_pending, slow_down, expired_token, access_denied).
func pollDeviceToken(ctx context.Context, cfg DeviceAuthConfig, deviceCode string) (*TokenResponse, error) {
	vals := url.Values{}
	vals.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	vals.Set("device_code", deviceCode)
	vals.Set("client_id", cfg.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthDeviceBuildTokenRequest, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := authHTTPClient.Do(req)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthDeviceTokenRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthDeviceReadTokenResponse, err)
	}

	// Non-200 responses during polling carry structured error codes.
	if resp.StatusCode != http.StatusOK {
		var pollErr devicePollError
		if jsonErr := json.Unmarshal(body, &pollErr); jsonErr == nil && pollErr.ErrorCode != "" {
			return nil, &pollErr
		}
		return nil, i18n.NewError(i18n.KeyAuthDeviceTokenEndpointRejected, resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthDeviceDecodeTokenResponse, err)
	}
	if tr.ExpiresIn > 0 {
		tr.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return &tr, nil
}
