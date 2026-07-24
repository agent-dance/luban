package i18n

// Build identifiers and timestamps remain raw format values; only labels and
// comparison states are localized at the final user-visible surface.
const (
	KeyBuildFingerprintDetail Key = "build.fingerprint.detail"
	KeyBuildValueUnknown      Key = "build.value.unknown"
	KeyBuildStateClean        Key = "build.state.clean"
	KeyBuildStateDirty        Key = "build.state.dirty"
	KeyBuildStateUnknown      Key = "build.state.unknown"
	KeyBuildHeadMatch         Key = "build.head.match"
	KeyBuildHeadStale         Key = "build.head.stale"
	KeyBuildHeadUnknown       Key = "build.head.unknown"
)

var buildFingerprintKeys = [...]Key{
	KeyBuildFingerprintDetail, KeyBuildValueUnknown,
	KeyBuildStateClean, KeyBuildStateDirty, KeyBuildStateUnknown,
	KeyBuildHeadMatch, KeyBuildHeadStale, KeyBuildHeadUnknown,
}

func init() {
	addBuildFingerprint := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	addBuildFingerprint(KeyBuildFingerprintDetail,
		"\nBuild identity:\n  Version:         %s\n  Revision:        %s\n  State:           %s\n  Build time:      %s\n  Process started: %s\n  Executable:      %s\n  Repository HEAD: %s\n",
		"\n构建标识：\n  版本：           %s\n  修订：           %s\n  状态：           %s\n  构建时间：       %s\n  进程启动时间：   %s\n  可执行文件：     %s\n  仓库 HEAD：      %s\n",
		"\nBuild-Identität:\n  Version:         %s\n  Revision:        %s\n  Status:          %s\n  Build-Zeit:      %s\n  Prozessstart:    %s\n  Programmdatei:   %s\n  Repository-HEAD: %s\n",
		"\nビルド識別情報:\n  バージョン:       %s\n  リビジョン:       %s\n  状態:             %s\n  ビルド時刻:       %s\n  プロセス開始:     %s\n  実行ファイル:     %s\n  リポジトリ HEAD:  %s\n",
		"\n빌드 식별 정보:\n  버전:            %s\n  리비전:          %s\n  상태:            %s\n  빌드 시간:       %s\n  프로세스 시작:   %s\n  실행 파일:       %s\n  저장소 HEAD:     %s\n",
		"\nИдентификатор сборки:\n  Версия:          %s\n  Ревизия:         %s\n  Состояние:       %s\n  Время сборки:    %s\n  Запуск процесса: %s\n  Исполняемый файл:%s\n  HEAD репозитория:%s\n")
	addBuildFingerprint(KeyBuildValueUnknown,
		"unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно")
	addBuildFingerprint(KeyBuildStateClean,
		"clean build", "干净构建", "sauberer Build", "クリーンビルド", "클린 빌드", "чистая сборка")
	addBuildFingerprint(KeyBuildStateDirty,
		"dirty build", "脏构建", "veränderter Build", "変更ありのビルド", "변경된 빌드", "изменённая сборка")
	addBuildFingerprint(KeyBuildStateUnknown,
		"build state unknown", "构建状态未知", "Build-Status unbekannt", "ビルド状態不明", "빌드 상태 알 수 없음", "состояние сборки неизвестно")
	addBuildFingerprint(KeyBuildHeadMatch,
		"matches HEAD", "与 HEAD 匹配", "stimmt mit HEAD überein", "HEAD と一致", "HEAD와 일치", "совпадает с HEAD")
	addBuildFingerprint(KeyBuildHeadStale,
		"stale: does not match HEAD", "已过期：与 HEAD 不一致", "veraltet: stimmt nicht mit HEAD überein", "古い状態: HEAD と不一致", "오래된 상태: HEAD와 일치하지 않음", "устарела: не совпадает с HEAD")
	addBuildFingerprint(KeyBuildHeadUnknown,
		"HEAD comparison unknown", "无法确定与 HEAD 的关系", "HEAD-Vergleich unbekannt", "HEAD との比較不明", "HEAD 비교 알 수 없음", "сравнение с HEAD невозможно")
}
