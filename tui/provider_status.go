package tui

// ProviderStatus represents the connection state of the active provider.
type ProviderStatus int

const (
	// StatusUnknown is the default state before any check.
	StatusUnknown ProviderStatus = iota
	// StatusConnected indicates the provider has valid credentials and is reachable.
	StatusConnected
	// StatusDisconnected indicates no credentials are configured.
	StatusDisconnected
	// StatusError indicates credentials exist but the provider returned an error.
	StatusError
)

// String returns a human-readable label for the status.
func (s ProviderStatus) String() string {
	switch s {
	case StatusConnected:
		return "connected"
	case StatusDisconnected:
		return "disconnected"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// Badge returns a colored emoji badge for the status.
func (s ProviderStatus) Badge() string {
	switch s {
	case StatusConnected:
		return "🟢"
	case StatusDisconnected:
		return "⚪"
	case StatusError:
		return "🔴"
	default:
		return "⚪"
	}
}
