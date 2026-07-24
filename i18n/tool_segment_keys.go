package i18n

func init() {
	segment := func(en, zh, de, ja, ko, ru string) map[Language]string {
		return map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	for key, translations := range map[Key]map[Language]string{
		KeyToolSegmentReadFiles: segment(
			"Read files",
			"已读取文件",
			"Dateien gelesen",
			"ファイルを読み取りました",
			"파일 읽음",
			"Файлы прочитаны",
		),
		KeyToolSegmentUsedTools: segment(
			"Used %d tools",
			"已使用 %d 个工具",
			"%d Tools verwendet",
			"%d 個のツールを使用しました",
			"도구 %d개 사용",
			"Использовано инструментов: %d",
		),
		KeyToolSegmentIssues: segment(
			"%s — %d issues",
			"%s — %d 项异常",
			"%s — %d Probleme",
			"%s — 問題 %d 件",
			"%s — 문제 %d건",
			"%s — проблем: %d",
		),
	} {
		semanticTranslations[key] = translations
	}
}
