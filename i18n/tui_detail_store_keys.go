package i18n

// Semantic errors emitted while retaining and restoring exact TUI evidence.
// Logical keys, sources, digests, journal entries, filesystem paths, and raw
// OS/JSON causes remain formatting parameters.
const (
	KeyTUIDetailStoreNotFound             Key = "tui.detail_store.not_found"
	KeyTUIDetailStoreInvalidReference     Key = "tui.detail_store.invalid_reference"
	KeyTUIDetailStoreNotFoundKey          Key = "tui.detail_store.not_found_key"
	KeyTUIDetailStoreRetainedIntegrity    Key = "tui.detail_store.retained_integrity"
	KeyTUIDetailStoreRetainedDigest       Key = "tui.detail_store.retained_digest"
	KeyTUIDetailStoreArtifactRootEmpty    Key = "tui.detail_store.artifact_root_empty"
	KeyTUIDetailStoreResolveArtifactRoot  Key = "tui.detail_store.resolve_artifact_root"
	KeyTUIDetailStorePrepareArtifactRoot  Key = "tui.detail_store.prepare_artifact_root"
	KeyTUIDetailStorePrepareShard         Key = "tui.detail_store.prepare_shard"
	KeyTUIDetailStoreCreateTemporary      Key = "tui.detail_store.create_temporary"
	KeyTUIDetailStoreSecureTemporary      Key = "tui.detail_store.secure_temporary"
	KeyTUIDetailStoreWrite                Key = "tui.detail_store.write"
	KeyTUIDetailStoreSync                 Key = "tui.detail_store.sync"
	KeyTUIDetailStoreClose                Key = "tui.detail_store.close"
	KeyTUIDetailStorePublish              Key = "tui.detail_store.publish"
	KeyTUIDetailStoreSyncDirectory        Key = "tui.detail_store.sync_directory"
	KeyTUIDetailStoreJournalInvalid       Key = "tui.detail_store.journal.invalid"
	KeyTUIDetailStoreJournalReference     Key = "tui.detail_store.journal.reference"
	KeyTUIDetailStoreEncodeJournal        Key = "tui.detail_store.journal.encode"
	KeyTUIDetailStorePrepareJournal       Key = "tui.detail_store.journal.prepare"
	KeyTUIDetailStorePublishJournal       Key = "tui.detail_store.journal.publish"
	KeyTUIDetailStoreSyncJournal          Key = "tui.detail_store.journal.sync"
	KeyTUIDetailStoreReadJournal          Key = "tui.detail_store.journal.read"
	KeyTUIDetailStoreInspectJournalEntry  Key = "tui.detail_store.journal.inspect_entry"
	KeyTUIDetailStoreJournalEntryInvalid  Key = "tui.detail_store.journal.entry_invalid"
	KeyTUIDetailStoreReadJournalEntry     Key = "tui.detail_store.journal.read_entry"
	KeyTUIDetailStoreDecodeJournalEntry   Key = "tui.detail_store.journal.decode_entry"
	KeyTUIDetailStoreValidateJournal      Key = "tui.detail_store.journal.validate"
	KeyTUIDetailStoreInspectDetail        Key = "tui.detail_store.inspect_detail"
	KeyTUIDetailStoreDetailNotRegular     Key = "tui.detail_store.detail_not_regular"
	KeyTUIDetailStoreDetailPermissions    Key = "tui.detail_store.detail_permissions"
	KeyTUIDetailStoreOpenDetail           Key = "tui.detail_store.open_detail"
	KeyTUIDetailStoreStatDetail           Key = "tui.detail_store.stat_detail"
	KeyTUIDetailStoreDetailChanged        Key = "tui.detail_store.detail_changed"
	KeyTUIDetailStoreDetailSize           Key = "tui.detail_store.detail_size"
	KeyTUIDetailStoreReadDetail           Key = "tui.detail_store.read_detail"
	KeyTUIDetailStoreDetailIntegrity      Key = "tui.detail_store.detail_integrity"
	KeyTUIDetailStoreDetailDigest         Key = "tui.detail_store.detail_digest"
	KeyTUIDetailStoreRelativizePath       Key = "tui.detail_store.relativize_path"
	KeyTUIDetailStorePathEscapesRoot      Key = "tui.detail_store.path_escapes_root"
	KeyTUIDetailStoreLogicalKeyInvalid    Key = "tui.detail_store.logical_key_invalid"
	KeyTUIDetailStoreSourceMismatch       Key = "tui.detail_store.source_mismatch"
	KeyTUIDetailStoreReferenceMalformed   Key = "tui.detail_store.reference_malformed"
	KeyTUIDetailStoreDigestMalformed      Key = "tui.detail_store.digest_malformed"
	KeyTUIDetailStorePathNotRealDirectory Key = "tui.detail_store.path_not_real_directory"
)

var tuiDetailStoreKeys = [...]Key{
	KeyTUIDetailStoreNotFound,
	KeyTUIDetailStoreInvalidReference,
	KeyTUIDetailStoreNotFoundKey,
	KeyTUIDetailStoreRetainedIntegrity,
	KeyTUIDetailStoreRetainedDigest,
	KeyTUIDetailStoreArtifactRootEmpty,
	KeyTUIDetailStoreResolveArtifactRoot,
	KeyTUIDetailStorePrepareArtifactRoot,
	KeyTUIDetailStorePrepareShard,
	KeyTUIDetailStoreCreateTemporary,
	KeyTUIDetailStoreSecureTemporary,
	KeyTUIDetailStoreWrite,
	KeyTUIDetailStoreSync,
	KeyTUIDetailStoreClose,
	KeyTUIDetailStorePublish,
	KeyTUIDetailStoreSyncDirectory,
	KeyTUIDetailStoreJournalInvalid,
	KeyTUIDetailStoreJournalReference,
	KeyTUIDetailStoreEncodeJournal,
	KeyTUIDetailStorePrepareJournal,
	KeyTUIDetailStorePublishJournal,
	KeyTUIDetailStoreSyncJournal,
	KeyTUIDetailStoreReadJournal,
	KeyTUIDetailStoreInspectJournalEntry,
	KeyTUIDetailStoreJournalEntryInvalid,
	KeyTUIDetailStoreReadJournalEntry,
	KeyTUIDetailStoreDecodeJournalEntry,
	KeyTUIDetailStoreValidateJournal,
	KeyTUIDetailStoreInspectDetail,
	KeyTUIDetailStoreDetailNotRegular,
	KeyTUIDetailStoreDetailPermissions,
	KeyTUIDetailStoreOpenDetail,
	KeyTUIDetailStoreStatDetail,
	KeyTUIDetailStoreDetailChanged,
	KeyTUIDetailStoreDetailSize,
	KeyTUIDetailStoreReadDetail,
	KeyTUIDetailStoreDetailIntegrity,
	KeyTUIDetailStoreDetailDigest,
	KeyTUIDetailStoreRelativizePath,
	KeyTUIDetailStorePathEscapesRoot,
	KeyTUIDetailStoreLogicalKeyInvalid,
	KeyTUIDetailStoreSourceMismatch,
	KeyTUIDetailStoreReferenceMalformed,
	KeyTUIDetailStoreDigestMalformed,
	KeyTUIDetailStorePathNotRealDirectory,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyTUIDetailStoreNotFound,
		"detail not found",
		"找不到证据详情",
		"Detail wurde nicht gefunden",
		"証拠の詳細が見つかりません",
		"증거 세부 정보를 찾을 수 없습니다",
		"Детальные данные не найдены")
	add(KeyTUIDetailStoreInvalidReference,
		"invalid detail reference",
		"无效的证据详情引用",
		"Ungültiger Detailverweis",
		"証拠の詳細への参照が無効です",
		"증거 세부 정보 참조가 올바르지 않습니다",
		"Недопустимая ссылка на детальные данные")
	add(KeyTUIDetailStoreNotFoundKey,
		"detail not found: %s",
		"找不到证据详情：%s",
		"Detail wurde nicht gefunden: %s",
		"証拠の詳細が見つかりません：%s",
		"증거 세부 정보를 찾을 수 없습니다: %s",
		"Детальные данные не найдены: %s")
	add(KeyTUIDetailStoreRetainedIntegrity,
		"invalid detail reference: retained detail %q failed integrity check (size %d, expected %d)",
		"无效的证据详情引用：已保留的详情 %q 未通过完整性检查（大小为 %d，预期为 %d）",
		"Ungültiger Detailverweis: Das gespeicherte Detail %q hat die Integritätsprüfung nicht bestanden (Größe %d, erwartet %d)",
		"証拠の詳細への参照が無効です：保持された詳細 %q の整合性チェックに失敗しました（サイズ %d、期待値 %d）",
		"증거 세부 정보 참조가 올바르지 않습니다: 보관된 세부 정보 %q이(가) 무결성 검사를 통과하지 못했습니다(크기 %d, 예상 %d)",
		"Недопустимая ссылка на детальные данные: сохранённые данные %q не прошли проверку целостности (размер %d, ожидался %d)")
	add(KeyTUIDetailStoreRetainedDigest,
		"invalid detail reference: retained detail %q digest %q differs from reference %q",
		"无效的证据详情引用：已保留的详情 %q 摘要 %q 与引用中的 %q 不一致",
		"Ungültiger Detailverweis: Der Digest %[2]q des gespeicherten Details %[1]q weicht vom Verweis %[3]q ab",
		"証拠の詳細への参照が無効です：保持された詳細 %q のダイジェスト %q が参照 %q と一致しません",
		"증거 세부 정보 참조가 올바르지 않습니다: 보관된 세부 정보 %q의 다이제스트 %q이(가) 참조 %q과(와) 다릅니다",
		"Недопустимая ссылка на детальные данные: дайджест %[2]q сохранённых данных %[1]q отличается от ссылки %[3]q")
	add(KeyTUIDetailStoreArtifactRootEmpty,
		"artifact root: invalid detail reference",
		"制品根目录：无效的证据详情引用",
		"Artefaktstamm: ungültiger Detailverweis",
		"アーティファクトのルート：証拠の詳細への参照が無効です",
		"아티팩트 루트: 증거 세부 정보 참조가 올바르지 않습니다",
		"Корень артефактов: недопустимая ссылка на детальные данные")
	add(KeyTUIDetailStoreResolveArtifactRoot,
		"resolve artifact root %q: %v",
		"解析制品根目录 %q 失败：%v",
		"Artefaktstamm %q konnte nicht aufgelöst werden: %v",
		"アーティファクトのルート %q を解決できませんでした：%v",
		"아티팩트 루트 %q을(를) 확인하지 못했습니다: %v",
		"Не удалось разрешить корень артефактов %q: %v")
	add(KeyTUIDetailStorePrepareArtifactRoot,
		"prepare artifact root %q: %v",
		"准备制品根目录 %q 失败：%v",
		"Artefaktstamm %q konnte nicht vorbereitet werden: %v",
		"アーティファクトのルート %q を準備できませんでした：%v",
		"아티팩트 루트 %q을(를) 준비하지 못했습니다: %v",
		"Не удалось подготовить корень артефактов %q: %v")
	add(KeyTUIDetailStorePrepareShard,
		"prepare detail shard %q: %v",
		"准备证据详情分片 %q 失败：%v",
		"Detail-Shard %q konnte nicht vorbereitet werden: %v",
		"証拠の詳細シャード %q を準備できませんでした：%v",
		"증거 세부 정보 샤드 %q을(를) 준비하지 못했습니다: %v",
		"Не удалось подготовить сегмент детальных данных %q: %v")
	add(KeyTUIDetailStoreCreateTemporary,
		"create detail temporary file in %q: %v",
		"在 %q 中创建证据详情临时文件失败：%v",
		"Temporäre Detaildatei in %q konnte nicht erstellt werden: %v",
		"%q に証拠の詳細用一時ファイルを作成できませんでした：%v",
		"%q에 증거 세부 정보 임시 파일을 만들지 못했습니다: %v",
		"Не удалось создать временный файл детальных данных в %q: %v")
	add(KeyTUIDetailStoreSecureTemporary,
		"secure detail temporary file %q: %v",
		"保护证据详情临时文件 %q 失败：%v",
		"Temporäre Detaildatei %q konnte nicht abgesichert werden: %v",
		"証拠の詳細用一時ファイル %q を保護できませんでした：%v",
		"증거 세부 정보 임시 파일 %q을(를) 보호하지 못했습니다: %v",
		"Не удалось защитить временный файл детальных данных %q: %v")
	add(KeyTUIDetailStoreWrite,
		"write detail %q: %v",
		"写入证据详情 %q 失败：%v",
		"Detail %q konnte nicht geschrieben werden: %v",
		"証拠の詳細 %q を書き込めませんでした：%v",
		"증거 세부 정보 %q을(를) 쓰지 못했습니다: %v",
		"Не удалось записать детальные данные %q: %v")
	add(KeyTUIDetailStoreSync,
		"sync detail %q: %v",
		"同步证据详情 %q 失败：%v",
		"Detail %q konnte nicht synchronisiert werden: %v",
		"証拠の詳細 %q を同期できませんでした：%v",
		"증거 세부 정보 %q을(를) 동기화하지 못했습니다: %v",
		"Не удалось синхронизировать детальные данные %q: %v")
	add(KeyTUIDetailStoreClose,
		"close detail %q: %v",
		"关闭证据详情文件 %q 失败：%v",
		"Detaildatei %q konnte nicht geschlossen werden: %v",
		"証拠の詳細ファイル %q を閉じられませんでした：%v",
		"증거 세부 정보 파일 %q을(를) 닫지 못했습니다: %v",
		"Не удалось закрыть файл детальных данных %q: %v")
	add(KeyTUIDetailStorePublish,
		"publish detail %q as %q: %v",
		"将证据详情 %q 发布为 %q 失败：%v",
		"Detail %q konnte nicht als %q veröffentlicht werden: %v",
		"証拠の詳細 %q を %q として公開できませんでした：%v",
		"증거 세부 정보 %q을(를) %q(으)로 게시하지 못했습니다: %v",
		"Не удалось опубликовать детальные данные %q как %q: %v")
	add(KeyTUIDetailStoreSyncDirectory,
		"sync detail directory %q: %v",
		"同步证据详情目录 %q 失败：%v",
		"Detailverzeichnis %q konnte nicht synchronisiert werden: %v",
		"証拠の詳細ディレクトリ %q を同期できませんでした：%v",
		"증거 세부 정보 디렉터리 %q을(를) 동기화하지 못했습니다: %v",
		"Не удалось синхронизировать каталог детальных данных %q: %v")
	add(KeyTUIDetailStoreJournalInvalid,
		"journal observation: invalid detail reference",
		"记录 observation：无效的证据详情引用",
		"Observation konnte nicht protokolliert werden: ungültiger Detailverweis",
		"observation を記録できません：証拠の詳細への参照が無効です",
		"observation 기록 실패: 증거 세부 정보 참조가 올바르지 않습니다",
		"Не удалось записать observation: недопустимая ссылка на детальные данные")
	add(KeyTUIDetailStoreJournalReference,
		"journal observation %s: %v",
		"记录 observation %s 失败：%v",
		"Observation %s konnte nicht protokolliert werden: %v",
		"observation %s を記録できませんでした：%v",
		"observation %s을(를) 기록하지 못했습니다: %v",
		"Не удалось записать observation %s: %v")
	add(KeyTUIDetailStoreEncodeJournal,
		"encode observation journal for %s: %v",
		"编码 observation %s 的日志失败：%v",
		"Journal für Observation %s konnte nicht kodiert werden: %v",
		"observation %s のジャーナルをエンコードできませんでした：%v",
		"observation %s의 저널을 인코딩하지 못했습니다: %v",
		"Не удалось закодировать журнал observation %s: %v")
	add(KeyTUIDetailStorePrepareJournal,
		"prepare observation journal %q: %v",
		"准备 observation 日志目录 %q 失败：%v",
		"Observation-Journal %q konnte nicht vorbereitet werden: %v",
		"observation ジャーナル %q を準備できませんでした：%v",
		"observation 저널 %q을(를) 준비하지 못했습니다: %v",
		"Не удалось подготовить журнал observation %q: %v")
	add(KeyTUIDetailStorePublishJournal,
		"publish observation journal %q: %v",
		"发布 observation 日志 %q 失败：%v",
		"Observation-Journal %q konnte nicht veröffentlicht werden: %v",
		"observation ジャーナル %q を公開できませんでした：%v",
		"observation 저널 %q을(를) 게시하지 못했습니다: %v",
		"Не удалось опубликовать журнал observation %q: %v")
	add(KeyTUIDetailStoreSyncJournal,
		"sync observation journal %q: %v",
		"同步 observation 日志目录 %q 失败：%v",
		"Observation-Journal %q konnte nicht synchronisiert werden: %v",
		"observation ジャーナル %q を同期できませんでした：%v",
		"observation 저널 %q을(를) 동기화하지 못했습니다: %v",
		"Не удалось синхронизировать журнал observation %q: %v")
	add(KeyTUIDetailStoreReadJournal,
		"read observation journal %q: %v",
		"读取 observation 日志目录 %q 失败：%v",
		"Observation-Journal %q konnte nicht gelesen werden: %v",
		"observation ジャーナル %q を読み込めませんでした：%v",
		"observation 저널 %q을(를) 읽지 못했습니다: %v",
		"Не удалось прочитать журнал observation %q: %v")
	add(KeyTUIDetailStoreInspectJournalEntry,
		"inspect observation journal entry %s at %q: %v",
		"检查 observation 日志条目 %s（%q）失败：%v",
		"Observation-Journaleintrag %s unter %q konnte nicht geprüft werden: %v",
		"observation ジャーナルエントリ %s（%q）を検査できませんでした：%v",
		"observation 저널 항목 %s(%q)을(를) 검사하지 못했습니다: %v",
		"Не удалось проверить запись журнала observation %s по пути %q: %v")
	add(KeyTUIDetailStoreJournalEntryInvalid,
		"observation journal entry %s at %q: invalid detail reference",
		"observation 日志条目 %s（%q）：无效的证据详情引用",
		"Observation-Journaleintrag %s unter %q: ungültiger Detailverweis",
		"observation ジャーナルエントリ %s（%q）：証拠の詳細への参照が無効です",
		"observation 저널 항목 %s(%q): 증거 세부 정보 참조가 올바르지 않습니다",
		"Запись журнала observation %s по пути %q: недопустимая ссылка на детальные данные")
	add(KeyTUIDetailStoreReadJournalEntry,
		"read observation journal entry %s at %q: %v",
		"读取 observation 日志条目 %s（%q）失败：%v",
		"Observation-Journaleintrag %s unter %q konnte nicht gelesen werden: %v",
		"observation ジャーナルエントリ %s（%q）を読み込めませんでした：%v",
		"observation 저널 항목 %s(%q)을(를) 읽지 못했습니다: %v",
		"Не удалось прочитать запись журнала observation %s по пути %q: %v")
	add(KeyTUIDetailStoreDecodeJournalEntry,
		"decode observation journal entry %s: %v",
		"解码 observation 日志条目 %s 失败：%v",
		"Observation-Journaleintrag %s konnte nicht dekodiert werden: %v",
		"observation ジャーナルエントリ %s をデコードできませんでした：%v",
		"observation 저널 항목 %s을(를) 디코딩하지 못했습니다: %v",
		"Не удалось декодировать запись журнала observation %s: %v")
	add(KeyTUIDetailStoreValidateJournal,
		"validate observation journal %s: %v",
		"验证 observation 日志 %s 失败：%v",
		"Observation-Journal %s konnte nicht validiert werden: %v",
		"observation ジャーナル %s を検証できませんでした：%v",
		"observation 저널 %s을(를) 검증하지 못했습니다: %v",
		"Не удалось проверить журнал observation %s: %v")
	add(KeyTUIDetailStoreInspectDetail,
		"inspect detail %q: %v",
		"检查证据详情 %q 失败：%v",
		"Detail %q konnte nicht geprüft werden: %v",
		"証拠の詳細 %q を検査できませんでした：%v",
		"증거 세부 정보 %q을(를) 검사하지 못했습니다: %v",
		"Не удалось проверить детальные данные %q: %v")
	add(KeyTUIDetailStoreDetailNotRegular,
		"invalid detail reference: detail %q is not a regular file",
		"无效的证据详情引用：%q 不是常规文件",
		"Ungültiger Detailverweis: %q ist keine reguläre Datei",
		"証拠の詳細への参照が無効です：%q は通常のファイルではありません",
		"증거 세부 정보 참조가 올바르지 않습니다: %q은(는) 일반 파일이 아닙니다",
		"Недопустимая ссылка на детальные данные: %q не является обычным файлом")
	add(KeyTUIDetailStoreDetailPermissions,
		"invalid detail reference: detail %q permissions %s are not private",
		"无效的证据详情引用：%q 的权限 %s 并非私有",
		"Ungültiger Detailverweis: Die Berechtigungen %[2]s von %[1]q sind nicht privat",
		"証拠の詳細への参照が無効です：%q のパーミッション %s は非公開になっていません",
		"증거 세부 정보 참조가 올바르지 않습니다: %q의 권한 %s이(가) 비공개가 아닙니다",
		"Недопустимая ссылка на детальные данные: права %[2]s для %[1]q не являются закрытыми")
	add(KeyTUIDetailStoreOpenDetail,
		"open detail %q: %v",
		"打开证据详情 %q 失败：%v",
		"Detail %q konnte nicht geöffnet werden: %v",
		"証拠の詳細 %q を開けませんでした：%v",
		"증거 세부 정보 %q을(를) 열지 못했습니다: %v",
		"Не удалось открыть детальные данные %q: %v")
	add(KeyTUIDetailStoreStatDetail,
		"stat detail %q: %v",
		"读取证据详情 %q 的文件信息失败：%v",
		"Dateiinformationen für Detail %q konnten nicht gelesen werden: %v",
		"証拠の詳細 %q のファイル情報を取得できませんでした：%v",
		"증거 세부 정보 %q의 파일 정보를 가져오지 못했습니다: %v",
		"Не удалось получить сведения о детальных данных %q: %v")
	add(KeyTUIDetailStoreDetailChanged,
		"invalid detail reference: detail %q changed while opening",
		"无效的证据详情引用：%q 在打开过程中发生了变化",
		"Ungültiger Detailverweis: Detail %q wurde beim Öffnen verändert",
		"証拠の詳細への参照が無効です：%q は開いている間に変更されました",
		"증거 세부 정보 참조가 올바르지 않습니다: %q을(를) 여는 동안 변경되었습니다",
		"Недопустимая ссылка на детальные данные: %q изменились во время открытия")
	add(KeyTUIDetailStoreDetailSize,
		"invalid detail reference: detail %q size %d differs from reference %d",
		"无效的证据详情引用：%q 的大小 %d 与引用中的 %d 不一致",
		"Ungültiger Detailverweis: Die Größe %[2]d von Detail %[1]q weicht vom Verweis %[3]d ab",
		"証拠の詳細への参照が無効です：%q のサイズ %d が参照 %d と一致しません",
		"증거 세부 정보 참조가 올바르지 않습니다: %q의 크기 %d이(가) 참조 %d과(와) 다릅니다",
		"Недопустимая ссылка на детальные данные: размер %[2]d для %[1]q отличается от ссылки %[3]d")
	add(KeyTUIDetailStoreReadDetail,
		"read detail %q: %v",
		"读取证据详情 %q 失败：%v",
		"Detail %q konnte nicht gelesen werden: %v",
		"証拠の詳細 %q を読み込めませんでした：%v",
		"증거 세부 정보 %q을(를) 읽지 못했습니다: %v",
		"Не удалось прочитать детальные данные %q: %v")
	add(KeyTUIDetailStoreDetailIntegrity,
		"invalid detail reference: detail %q failed integrity check (size %d, expected %d)",
		"无效的证据详情引用：%q 未通过完整性检查（大小为 %d，预期为 %d）",
		"Ungültiger Detailverweis: Detail %q hat die Integritätsprüfung nicht bestanden (Größe %d, erwartet %d)",
		"証拠の詳細への参照が無効です：%q の整合性チェックに失敗しました（サイズ %d、期待値 %d）",
		"증거 세부 정보 참조가 올바르지 않습니다: %q이(가) 무결성 검사를 통과하지 못했습니다(크기 %d, 예상 %d)",
		"Недопустимая ссылка на детальные данные: %q не прошли проверку целостности (размер %d, ожидался %d)")
	add(KeyTUIDetailStoreDetailDigest,
		"invalid detail reference: detail %q digest %q differs from reference %q",
		"无效的证据详情引用：%q 的摘要 %q 与引用中的 %q 不一致",
		"Ungültiger Detailverweis: Der Digest %[2]q von Detail %[1]q weicht vom Verweis %[3]q ab",
		"証拠の詳細への参照が無効です：%q のダイジェスト %q が参照 %q と一致しません",
		"증거 세부 정보 참조가 올바르지 않습니다: %q의 다이제스트 %q이(가) 참조 %q과(와) 다릅니다",
		"Недопустимая ссылка на детальные данные: дайджест %[2]q для %[1]q отличается от ссылки %[3]q")
	add(KeyTUIDetailStoreRelativizePath,
		"validate detail path %q against artifact root %q: %v",
		"验证证据详情路径 %q 是否位于制品根目录 %q 时失败：%v",
		"Detailpfad %q konnte nicht gegen den Artefaktstamm %q geprüft werden: %v",
		"証拠の詳細パス %q をアーティファクトのルート %q に対して検証できませんでした：%v",
		"증거 세부 정보 경로 %q을(를) 아티팩트 루트 %q 기준으로 검증하지 못했습니다: %v",
		"Не удалось проверить путь детальных данных %q относительно корня артефактов %q: %v")
	add(KeyTUIDetailStorePathEscapesRoot,
		"invalid detail reference: detail path %q escapes artifact root %q",
		"无效的证据详情引用：详情路径 %q 超出了制品根目录 %q",
		"Ungültiger Detailverweis: Der Detailpfad %q verlässt den Artefaktstamm %q",
		"証拠の詳細への参照が無効です：詳細パス %q がアーティファクトのルート %q の外を指しています",
		"증거 세부 정보 참조가 올바르지 않습니다: 세부 정보 경로 %q이(가) 아티팩트 루트 %q을(를) 벗어납니다",
		"Недопустимая ссылка на детальные данные: путь %q выходит за корень артефактов %q")
	add(KeyTUIDetailStoreLogicalKeyInvalid,
		"logical key %q: invalid detail reference",
		"逻辑键 %q：无效的证据详情引用",
		"Logischer Schlüssel %q: ungültiger Detailverweis",
		"論理キー %q：証拠の詳細への参照が無効です",
		"논리 키 %q: 증거 세부 정보 참조가 올바르지 않습니다",
		"Логический ключ %q: недопустимая ссылка на детальные данные")
	add(KeyTUIDetailStoreSourceMismatch,
		"invalid detail reference: source %q does not match store %q",
		"无效的证据详情引用：来源 %q 与存储 %q 不匹配",
		"Ungültiger Detailverweis: Quelle %q stimmt nicht mit Speicher %q überein",
		"証拠の詳細への参照が無効です：ソース %q がストア %q と一致しません",
		"증거 세부 정보 참조가 올바르지 않습니다: 소스 %q이(가) 저장소 %q과(와) 일치하지 않습니다",
		"Недопустимая ссылка на детальные данные: источник %q не соответствует хранилищу %q")
	add(KeyTUIDetailStoreReferenceMalformed,
		"invalid detail reference: malformed key %q or size %d",
		"无效的证据详情引用：键 %q 或大小 %d 格式不正确",
		"Ungültiger Detailverweis: Schlüssel %q oder Größe %d ist fehlerhaft",
		"証拠の詳細への参照が無効です：キー %q またはサイズ %d が不正です",
		"증거 세부 정보 참조가 올바르지 않습니다: 키 %q 또는 크기 %d이(가) 잘못되었습니다",
		"Недопустимая ссылка на детальные данные: некорректный ключ %q или размер %d")
	add(KeyTUIDetailStoreDigestMalformed,
		"invalid detail reference: malformed digest %q",
		"无效的证据详情引用：摘要 %q 格式不正确",
		"Ungültiger Detailverweis: fehlerhafter Digest %q",
		"証拠の詳細への参照が無効です：ダイジェスト %q が不正です",
		"증거 세부 정보 참조가 올바르지 않습니다: 다이제스트 %q이(가) 잘못되었습니다",
		"Недопустимая ссылка на детальные данные: некорректный дайджест %q")
	add(KeyTUIDetailStorePathNotRealDirectory,
		"path %q is not a real directory",
		"路径 %q 不是实际目录",
		"Pfad %q ist kein echtes Verzeichnis",
		"パス %q は実体のあるディレクトリではありません",
		"경로 %q은(는) 실제 디렉터리가 아닙니다",
		"Путь %q не является реальным каталогом")
}
