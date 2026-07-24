package i18n

// Remaining services-layer MCP copy that can cross from an injected runtime
// dependency into the user-visible authentication flow. Protocol identifiers
// and implementation type names remain unchanged.
const (
	KeyServicesMCPOAuthMemoryTokenStoreNil Key = "services.mcp.oauth.memory_token_store_nil"
)

var servicesMCPRemainingKeys = []Key{
	KeyServicesMCPOAuthMemoryTokenStoreNil,
}

func init() {
	semanticTranslations[KeyServicesMCPOAuthMemoryTokenStoreNil] = map[Language]string{
		LangEN: "services/mcp: nil memory token store",
		LangZH: "services/mcp：memory token store 为 nil",
		LangDE: "services/mcp: memory token store ist nil",
		LangJA: "services/mcp：memory token store が nil です",
		LangKO: "services/mcp: memory token store가 nil입니다",
		LangRU: "services/mcp: memory token store равен nil",
	}
}
