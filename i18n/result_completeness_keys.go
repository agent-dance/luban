package i18n

// Semantic copy for typed result-completeness provenance. Raw provenance
// identifiers remain protocol values and are never used as translation keys.
const (
	KeyPresentationPaginationWarning        Key = "presentation.completeness.pagination"
	KeyPresentationSourceTruncatedWarning   Key = "presentation.completeness.source_truncated"
	KeyPresentationCaptureDroppedWarning    Key = "presentation.completeness.capture_dropped"
	KeyPresentationDisplayPreviewWarning    Key = "presentation.completeness.display_preview"
	KeyPresentationDisplayPreviewEvidence   Key = "presentation.completeness.display_preview_evidence"
	KeyPresentationUnknownTruncationWarning Key = "presentation.completeness.unknown_truncation"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyPresentationPaginationWarning,
		"This is a paginated result window; adjust offset or limit to retrieve omitted entries",
		"当前结果是分页窗口；请调整 offset 或 limit 获取省略项",
		"Dies ist ein paginiertes Ergebnisfenster; passe offset oder limit an, um ausgelassene Einträge abzurufen",
		"これはページ分割された結果です。省略項目を取得するには offset または limit を調整してください",
		"페이지로 나눈 결과입니다. 생략된 항목을 가져오려면 offset 또는 limit을 조정하세요",
		"Это окно результатов с пагинацией; измените offset или limit, чтобы получить пропущенные элементы")
	add(KeyPresentationSourceTruncatedWarning,
		"The source stopped before producing a complete result; complete evidence is unavailable",
		"来源在生成完整结果前停止；完整证据不可用",
		"Die Quelle endete vor dem vollständigen Ergebnis; vollständige Belege sind nicht verfügbar",
		"ソースが完全な結果を生成する前に停止したため、完全な証拠はありません",
		"소스가 전체 결과를 만들기 전에 중단되어 완전한 근거를 사용할 수 없습니다",
		"Источник остановился до получения полного результата; полные данные недоступны")
	add(KeyPresentationCaptureDroppedWarning,
		"The capture limit discarded part of the result; complete evidence is unavailable",
		"采集上限已丢弃部分结果；完整证据不可用",
		"Das Erfassungslimit hat einen Teil des Ergebnisses verworfen; vollständige Belege sind nicht verfügbar",
		"取得上限により結果の一部が破棄されたため、完全な証拠はありません",
		"수집 한도로 결과 일부가 버려져 완전한 근거를 사용할 수 없습니다",
		"Лимит захвата отбросил часть результата; полные данные недоступны")
	add(KeyPresentationDisplayPreviewWarning,
		"The displayed result is a shortened preview",
		"当前显示的是缩短预览",
		"Das angezeigte Ergebnis ist eine gekürzte Vorschau",
		"表示中の結果は短縮プレビューです",
		"표시된 결과는 축약된 미리보기입니다",
		"Показан сокращённый предварительный просмотр")
	add(KeyPresentationDisplayPreviewEvidence,
		"The displayed result is a shortened preview; complete evidence is available",
		"当前显示的是缩短预览；完整证据可用",
		"Das angezeigte Ergebnis ist eine gekürzte Vorschau; vollständige Belege sind verfügbar",
		"表示中の結果は短縮プレビューです。完全な証拠を確認できます",
		"표시된 결과는 축약된 미리보기이며 전체 근거를 사용할 수 있습니다",
		"Показан сокращённый предварительный просмотр; полные данные доступны")
	add(KeyPresentationUnknownTruncationWarning,
		"The result is incomplete, but its truncation provenance is unavailable; complete evidence is not claimed",
		"结果不完整，但缺少截断来源；不声明完整证据可用",
		"Das Ergebnis ist unvollständig, aber die Kürzungsherkunft fehlt; vollständige Belege werden nicht zugesichert",
		"結果は不完全ですが切り詰め元が不明なため、完全な証拠があるとは表示しません",
		"결과가 불완전하지만 잘림 출처를 알 수 없어 완전한 근거가 있다고 표시하지 않습니다",
		"Результат неполный, но происхождение усечения неизвестно; наличие полных данных не заявляется")
}
