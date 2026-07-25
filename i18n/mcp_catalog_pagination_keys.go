package i18n

const (
	KeyServicesMCPCatalogCursorLoop Key = "services.mcp.catalog.cursor_loop"
	KeyServicesMCPCatalogPageLimit  Key = "services.mcp.catalog.page_limit"
	KeyServicesMCPCatalogItemLimit  Key = "services.mcp.catalog.item_limit"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(
		KeyServicesMCPCatalogCursorLoop,
		"MCP: %s returned the pagination cursor %q more than once",
		"MCP：%s 多次返回了同一分页 cursor %q",
		"MCP: %s hat den Pagination-Cursor %q mehrfach zurückgegeben",
		"MCP：%s が同じ pagination cursor %q を複数回返しました",
		"MCP: %s에서 동일한 페이지네이션 cursor %q을(를) 두 번 이상 반환했습니다",
		"MCP: %s повторно вернул cursor пагинации %q",
	)
	add(
		KeyServicesMCPCatalogPageLimit,
		"MCP: %s exceeded the catalog page limit of %d",
		"MCP：%s 超过了 catalog 的 %d 页上限",
		"MCP: %s hat das Kataloglimit von %d Seiten überschritten",
		"MCP：%s が catalog の上限である %d ページを超えました",
		"MCP: %s에서 catalog 페이지 한도 %d을(를) 초과했습니다",
		"MCP: %s превысил ограничение каталога в %d страниц",
	)
	add(
		KeyServicesMCPCatalogItemLimit,
		"MCP: %s exceeded the catalog item limit of %d",
		"MCP：%s 超过了 catalog 的 %d 项上限",
		"MCP: %s hat das Kataloglimit von %d Einträgen überschritten",
		"MCP：%s が catalog の上限である %d 件を超えました",
		"MCP: %s에서 catalog 항목 한도 %d개를 초과했습니다",
		"MCP: %s превысил ограничение каталога в %d элементов",
	)
}
