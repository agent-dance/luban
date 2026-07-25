package manager

import mcpauth "github.com/agent-dance/luban/internal/mcp/auth"

func withTestNeedsAuthCache(cache *mcpauth.NeedsAuthCache) ManagerOption {
	return func(manager *Manager) {
		manager.needsAuthCache = cache
	}
}

func withTestTransportFactory(factory transportFactory) ManagerOption {
	return func(manager *Manager) {
		manager.transportFactory = factory
	}
}

func withTestReconnectPolicy(policy reconnectPolicy) ManagerOption {
	return func(manager *Manager) {
		manager.reconnectPolicy = policy.withDefaults()
	}
}
