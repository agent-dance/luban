package provider

// StreamTransportFallback is implemented by providers that can permanently
// switch a session away from a failed preferred streaming transport.
type StreamTransportFallback interface {
	TryFallbackTransport() (from, to string, activated bool)
}

// TryFallbackTransport resolves provider decorators and activates the next
// safe streaming transport, if one is available. Transport names are stable
// protocol identifiers intended for diagnostics and localized copy arguments.
func TryFallbackTransport(p Provider) (from, to string, activated bool) {
	switch current := p.(type) {
	case StreamTransportFallback:
		return current.TryFallbackTransport()
	case *RetryProvider:
		return TryFallbackTransport(current.inner)
	case *ProviderRef:
		return TryFallbackTransport(current.Get())
	default:
		return "", "", false
	}
}
