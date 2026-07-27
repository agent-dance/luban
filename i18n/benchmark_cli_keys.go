package i18n

// Semantic copy for the standalone Agentic Coding benchmark operator CLI.
// Subcommand names, paths, environment variables, model IDs, and JSON output
// are protocol values and remain untranslated.
const (
	KeyBenchmarkCLIUsage                     Key = "benchmark.cli.usage"
	KeyBenchmarkCLIManifestFlag              Key = "benchmark.cli.flag.manifest"
	KeyBenchmarkCLIBackendFlag               Key = "benchmark.cli.flag.backend_config"
	KeyBenchmarkCLIWorkDirFlag               Key = "benchmark.cli.flag.work_dir"
	KeyBenchmarkCLIExecuteFlag               Key = "benchmark.cli.flag.execute"
	KeyBenchmarkCLIFailed                    Key = "benchmark.cli.failed"
	KeyBenchmarkCLIExecuteRequired           Key = "benchmark.cli.execute_required"
	KeyBenchmarkCLIMissingConfig             Key = "benchmark.cli.missing_config"
	KeyBenchmarkCLIUnknownCommand            Key = "benchmark.cli.unknown_command"
	KeyBenchmarkCLIRunStateExists            Key = "benchmark.cli.run_state_exists"
	KeyBenchmarkCLIResumeMissing             Key = "benchmark.cli.resume_missing"
	KeyBenchmarkSourceInspectFailed          Key = "benchmark.source.inspect_failed"
	KeyBenchmarkSourceNotPristine            Key = "benchmark.source.not_pristine"
	KeyBenchmarkSourceMutatedDuringPreflight Key = "benchmark.source.mutated_during_preflight"
	KeyBenchmarkSourceMutatedDuringExecution Key = "benchmark.source.mutated_during_execution"
	KeyBenchmarkCodexV8CanaryPending         Key = "benchmark.codex_v8_canary.pending"
)

var benchmarkCLIKeys = []Key{
	KeyBenchmarkCLIUsage, KeyBenchmarkCLIManifestFlag, KeyBenchmarkCLIBackendFlag,
	KeyBenchmarkCLIWorkDirFlag, KeyBenchmarkCLIExecuteFlag, KeyBenchmarkCLIFailed,
	KeyBenchmarkCLIExecuteRequired, KeyBenchmarkCLIMissingConfig,
	KeyBenchmarkCLIUnknownCommand, KeyBenchmarkCLIRunStateExists,
	KeyBenchmarkCLIResumeMissing, KeyBenchmarkSourceInspectFailed,
	KeyBenchmarkSourceNotPristine, KeyBenchmarkSourceMutatedDuringPreflight,
	KeyBenchmarkSourceMutatedDuringExecution, KeyBenchmarkCodexV8CanaryPending,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyBenchmarkCLIUsage,
		"Usage: agenticbench <lock|preflight|oracle|run|resume|score|ledger> --manifest PATH --backend-config PATH --work-dir PATH [--execute]",
		"用法：agenticbench <lock|preflight|oracle|run|resume|score|ledger> --manifest 路径 --backend-config 路径 --work-dir 路径 [--execute]",
		"Aufruf: agenticbench <lock|preflight|oracle|run|resume|score|ledger> --manifest PFAD --backend-config PFAD --work-dir PFAD [--execute]",
		"使用法: agenticbench <lock|preflight|oracle|run|resume|score|ledger> --manifest パス --backend-config パス --work-dir パス [--execute]",
		"사용법: agenticbench <lock|preflight|oracle|run|resume|score|ledger> --manifest 경로 --backend-config 경로 --work-dir 경로 [--execute]",
		"Использование: agenticbench <lock|preflight|oracle|run|resume|score|ledger> --manifest ПУТЬ --backend-config ПУТЬ --work-dir ПУТЬ [--execute]")
	add(KeyBenchmarkCLIManifestFlag, "Path to the immutable benchmark manifest.", "不可变测评 manifest 的路径。", "Pfad zum unveränderlichen Benchmark-Manifest.", "不変のベンチマーク manifest へのパス。", "변경 불가능한 벤치마크 manifest 경로입니다.", "Путь к неизменяемому манифесту теста.")
	add(KeyBenchmarkCLIBackendFlag, "Path to the non-secret Pier backend configuration.", "不含密钥的 Pier 后端配置路径。", "Pfad zur geheimnisfreien Pier-Backend-Konfiguration.", "秘密情報を含まない Pier バックエンド設定へのパス。", "비밀이 없는 Pier 백엔드 설정 경로입니다.", "Путь к конфигурации Pier без секретов.")
	add(KeyBenchmarkCLIWorkDirFlag, "External directory that owns benchmark artifacts.", "用于保存测评产物的外部目录。", "Externes Verzeichnis für Benchmark-Artefakte.", "ベンチマーク成果物を保存する外部ディレクトリ。", "벤치마크 산출물을 보관할 외부 디렉터리입니다.", "Внешний каталог для артефактов теста.")
	add(KeyBenchmarkCLIExecuteFlag, "Authorize container, registry, or model-backed execution.", "授权容器、registry 或模型实际执行。", "Container-, Registry- oder modellgestützte Ausführung autorisieren.", "コンテナ、registry、またはモデルを使う実行を許可します。", "컨테이너, registry 또는 모델 기반 실행을 승인합니다.", "Разрешить выполнение с контейнерами, registry или моделью.")
	add(KeyBenchmarkCLIFailed, "Benchmark command failed: %v", "测评命令失败：%v", "Benchmark-Befehl fehlgeschlagen: %v", "ベンチマークコマンドが失敗しました: %v", "벤치마크 명령이 실패했습니다: %v", "Команда теста завершилась ошибкой: %v")
	add(KeyBenchmarkCLIExecuteRequired, "This operation is a dry run. Pass --execute to authorize external execution.", "当前仅为 dry run；传入 --execute 才会授权外部执行。", "Dies ist ein Probelauf. Mit --execute wird die externe Ausführung autorisiert.", "これは dry run です。外部実行を許可するには --execute を指定してください。", "현재 dry run입니다. 외부 실행을 승인하려면 --execute를 지정하세요.", "Это пробный запуск. Для внешнего выполнения укажите --execute.")
	add(KeyBenchmarkCLIMissingConfig, "The manifest, backend configuration, and work directory are required.", "必须提供 manifest、后端配置和工作目录。", "Manifest, Backend-Konfiguration und Arbeitsverzeichnis sind erforderlich.", "manifest、バックエンド設定、作業ディレクトリが必要です。", "manifest, 백엔드 설정 및 작업 디렉터리가 필요합니다.", "Требуются манифест, конфигурация backend и рабочий каталог.")
	add(KeyBenchmarkCLIUnknownCommand, "Unknown benchmark command: %s", "未知测评命令：%s", "Unbekannter Benchmark-Befehl: %s", "不明なベンチマークコマンドです: %s", "알 수 없는 벤치마크 명령: %s", "Неизвестная команда теста: %s")
	add(KeyBenchmarkCLIRunStateExists, "A run state already exists; use resume.", "运行状态已存在；请使用 resume。", "Ein Laufstatus existiert bereits; verwende resume.", "実行状態が既に存在します。resume を使用してください。", "실행 상태가 이미 있습니다. resume을 사용하세요.", "Состояние запуска уже существует; используйте resume.")
	add(KeyBenchmarkCLIResumeMissing, "No run state exists; use run.", "不存在运行状态；请使用 run。", "Es existiert kein Laufstatus; verwende run.", "実行状態がありません。run を使用してください。", "실행 상태가 없습니다. run을 사용하세요.", "Состояние запуска отсутствует; используйте run.")
	add(KeyBenchmarkSourceInspectFailed,
		"The pinned benchmark source root could not be inspected.",
		"无法检查已固定的测评源码根目录。",
		"Das fixierte Quellwurzelverzeichnis des Benchmarks konnte nicht geprüft werden.",
		"固定されたベンチマークのソースルートを検査できませんでした。",
		"고정된 벤치마크 소스 루트를 검사할 수 없습니다.",
		"Не удалось проверить закреплённый корневой каталог исходного кода теста.")
	add(KeyBenchmarkSourceNotPristine,
		"The pinned benchmark source root is not pristine; tracked changes, untracked files, and ignored files are forbidden.",
		"已固定的测评源码根目录并非纯净状态；禁止存在已跟踪改动、未跟踪文件或被忽略文件。",
		"Das fixierte Quellwurzelverzeichnis des Benchmarks ist nicht unverändert; Änderungen an versionierten Dateien sowie nicht versionierte oder ignorierte Dateien sind unzulässig.",
		"固定されたベンチマークのソースルートがクリーンではありません。追跡対象の変更、未追跡ファイル、無視対象ファイルは禁止されています。",
		"고정된 벤치마크 소스 루트가 깨끗하지 않습니다. 추적 파일 변경, 미추적 파일 및 무시된 파일은 허용되지 않습니다.",
		"Закреплённый корневой каталог исходного кода теста не является чистым: запрещены изменения отслеживаемых файлов, неотслеживаемые и игнорируемые файлы.")
	add(KeyBenchmarkSourceMutatedDuringPreflight,
		"The pinned benchmark source root changed during preflight.",
		"已固定的测评源码根目录在预检期间发生了变化。",
		"Das fixierte Quellwurzelverzeichnis des Benchmarks wurde während der Vorprüfung verändert.",
		"固定されたベンチマークのソースルートが事前検査中に変更されました。",
		"고정된 벤치마크 소스 루트가 사전 점검 중 변경되었습니다.",
		"Закреплённый корневой каталог исходного кода теста изменился во время предварительной проверки.")
	add(KeyBenchmarkSourceMutatedDuringExecution,
		"The pinned benchmark source root changed during execution.",
		"已固定的测评源码根目录在执行期间发生了变化。",
		"Das fixierte Quellwurzelverzeichnis des Benchmarks wurde während der Ausführung verändert.",
		"固定されたベンチマークのソースルートが実行中に変更されました。",
		"고정된 벤치마크 소스 루트가 실행 중 변경되었습니다.",
		"Закреплённый корневой каталог исходного кода теста изменился во время выполнения.")
	add(KeyBenchmarkCodexV8CanaryPending,
		"The formal Codex v8 canonical canary is pending authorization and pinning; benchmark preflight is disabled.",
		"正式 Codex v8 规范 canary 尚待授权并固定；测评预检已禁用。",
		"Der formale kanonische Codex-v8-Canary wartet auf Autorisierung und Fixierung; die Benchmark-Vorprüfung ist deaktiviert.",
		"正式な Codex v8 canonical canary は承認と固定を待っているため、ベンチマークの事前検査は無効です。",
		"정식 Codex v8 canonical canary가 승인 및 고정을 기다리고 있어 벤치마크 사전 점검이 비활성화되었습니다.",
		"Формальный канонический canary Codex v8 ожидает разрешения и закрепления; предварительная проверка теста отключена.")
}
