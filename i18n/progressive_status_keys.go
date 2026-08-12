package i18n

const (
	KeyTUIStatusCompactionCount              Key = "tui.status.compaction_count"
	KeyTUIStatusProgressiveSavings           Key = "tui.status.progressive_savings"
	KeyTUIStatusProgressivePending           Key = "tui.status.progressive_pending"
	KeyTUIStatusProgressiveSavingsAndPending Key = "tui.status.progressive_savings_and_pending"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyTUIStatusCompactionCount,
		"Compact×%d", "压缩×%d", "Kompakt×%d", "圧縮×%d", "압축×%d", "Сжатие×%d")
	add(KeyTUIStatusProgressiveSavings,
		"Progressive compression  ✓ saved %s", "渐进压缩  ✓已省%s", "Progressive Komprimierung  ✓ %s gespart", "段階圧縮  ✓%s削減", "점진 압축  ✓%s 절감", "Постепенное сжатие  ✓ сохранено %s")
	add(KeyTUIStatusProgressivePending,
		"Progressive compression  …%d items, est. save %s", "渐进压缩  …%d项预计省%s", "Progressive Komprimierung  …%d Elemente, ca. %s", "段階圧縮  …%d件、約%s削減", "점진 압축  …%d개, 약 %s 절감", "Постепенное сжатие  …%d, ожидается %s")
	add(KeyTUIStatusProgressiveSavingsAndPending,
		"Progressive compression  ✓ saved %s │ …%d items, est. save %s", "渐进压缩  ✓已省%s │ …%d项预计省%s", "Progressive Komprimierung  ✓ %s gespart │ …%d Elemente, ca. %s", "段階圧縮  ✓%s削減 │ …%d件、約%s削減", "점진 압축  ✓%s 절감 │ …%d개, 약 %s 절감", "Постепенное сжатие  ✓ сохранено %s │ …%d, ожидается %s")
}
