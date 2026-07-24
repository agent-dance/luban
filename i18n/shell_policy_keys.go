package i18n

// Semantic copy for the unified shell policy analyzer. Commands, variable
// names, paths, and parser diagnostics stay raw parameters where applicable.
const (
	KeyShellPolicyBlockRoot           Key = "shell.policy.block.root"
	KeyShellPolicyBlockHome           Key = "shell.policy.block.home"
	KeyShellPolicyBlockSystem         Key = "shell.policy.block.system"
	KeyShellPolicyBlockRawDevice      Key = "shell.policy.block.raw_device"
	KeyShellPolicyBlockProtected      Key = "shell.policy.block.protected"
	KeyShellPolicyBlockKnownPattern   Key = "shell.policy.block.known_pattern"
	KeyShellPolicyAskDynamicTarget    Key = "shell.policy.ask.dynamic_target"
	KeyShellPolicyAskDynamicFlags     Key = "shell.policy.ask.dynamic_flags"
	KeyShellPolicyAskCommandSubst     Key = "shell.policy.ask.command_substitution"
	KeyShellPolicyAskParseFailure     Key = "shell.policy.ask.parse_failure"
	KeyShellPolicyAskUnprovenTarget   Key = "shell.policy.ask.unproven_target"
	KeyShellPolicyAskDestructive      Key = "shell.policy.ask.destructive"
	KeyShellPolicyAskStructural       Key = "shell.policy.ask.structural"
	KeyShellPolicyAskUnrestrictedCode Key = "shell.policy.ask.unrestricted_code"
	KeyShellPolicyRemediationApprove  Key = "shell.policy.remediation.approve"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyShellPolicyBlockRoot,
		"Recursive deletion of the filesystem root is blocked.", "禁止递归删除文件系统根目录。", "Das rekursive Löschen des Dateisystem-Stammverzeichnisses ist gesperrt.", "ファイルシステムのルートを再帰的に削除する操作はブロックされます。", "파일 시스템 루트의 재귀 삭제는 차단됩니다.", "Рекурсивное удаление корня файловой системы запрещено.")
	add(KeyShellPolicyBlockHome,
		"Recursive deletion of the home directory is blocked.", "禁止递归删除 HOME 目录。", "Das rekursive Löschen des Home-Verzeichnisses ist gesperrt.", "HOME ディレクトリを再帰的に削除する操作はブロックされます。", "HOME 디렉터리의 재귀 삭제는 차단됩니다.", "Рекурсивное удаление домашнего каталога запрещено.")
	add(KeyShellPolicyBlockSystem,
		"Recursive deletion of system path %s is blocked.", "禁止递归删除系统路径 %s。", "Das rekursive Löschen des Systempfads %s ist gesperrt.", "システムパス %s を再帰的に削除する操作はブロックされます。", "시스템 경로 %s의 재귀 삭제는 차단됩니다.", "Рекурсивное удаление системного пути %s запрещено.")
	add(KeyShellPolicyBlockRawDevice,
		"Direct access to raw device %s is blocked.", "禁止直接操作原始设备 %s。", "Der direkte Zugriff auf das Rohgerät %s ist gesperrt.", "raw device %s への直接アクセスはブロックされます。", "원시 장치 %s에 대한 직접 접근은 차단됩니다.", "Прямой доступ к необработанному устройству %s запрещён.")
	add(KeyShellPolicyBlockProtected,
		"Destructive access to protected path %s is blocked.", "禁止对受保护路径 %s 执行破坏性操作。", "Der destruktive Zugriff auf den geschützten Pfad %s ist gesperrt.", "保護されたパス %s への破壊的アクセスはブロックされます。", "보호된 경로 %s에 대한 파괴적 접근은 차단됩니다.", "Разрушительный доступ к защищённому пути %s запрещён.")
	add(KeyShellPolicyBlockKnownPattern,
		"The command matches a blocked safety pattern: %s", "命令匹配被禁止的安全模式：%s", "Der Befehl entspricht einem gesperrten Sicherheitsmuster: %s", "コマンドがブロック対象の安全パターンに一致します: %s", "명령이 차단된 안전 패턴과 일치합니다: %s", "Команда соответствует запрещённому шаблону безопасности: %s")
	add(KeyShellPolicyAskDynamicTarget,
		"The destructive target uses unresolved variable %s and requires approval.", "破坏性操作的目标使用了无法解析的变量 %s，需要批准。", "Das destruktive Ziel verwendet die nicht aufgelöste Variable %s und erfordert eine Genehmigung.", "破壊的操作の対象に未解決の変数 %s が使われているため、承認が必要です。", "파괴적 작업 대상에 확인되지 않은 변수 %s이(가) 사용되어 승인이 필요합니다.", "Цель разрушительной операции использует неразрешённую переменную %s и требует подтверждения.")
	add(KeyShellPolicyAskDynamicFlags,
		"Dynamic command flags cannot be verified and require approval.", "动态命令参数无法验证，需要批准。", "Dynamische Befehlsoptionen können nicht überprüft werden und erfordern eine Genehmigung.", "動的なコマンドオプションを検証できないため、承認が必要です。", "동적 명령 플래그를 검증할 수 없어 승인이 필요합니다.", "Динамические флаги команды невозможно проверить; требуется подтверждение.")
	add(KeyShellPolicyAskCommandSubst,
		"A destructive target is produced by command substitution and requires approval.", "破坏性操作的目标由命令替换生成，需要批准。", "Ein destruktives Ziel wird durch Befehlssubstitution erzeugt und erfordert eine Genehmigung.", "破壊的操作の対象がコマンド置換で生成されるため、承認が必要です。", "파괴적 작업 대상이 명령 대체로 생성되어 승인이 필요합니다.", "Цель разрушительной операции формируется подстановкой команды и требует подтверждения.")
	add(KeyShellPolicyAskParseFailure,
		"The shell command could not be analyzed reliably and requires approval.", "无法可靠分析此 shell 命令，需要批准。", "Der Shell-Befehl konnte nicht zuverlässig analysiert werden und erfordert eine Genehmigung.", "shell コマンドを確実に解析できないため、承認が必要です。", "shell 명령을 신뢰성 있게 분석할 수 없어 승인이 필요합니다.", "Команду shell не удалось надёжно проанализировать; требуется подтверждение.")
	add(KeyShellPolicyAskUnprovenTarget,
		"Destructive target %s is outside the proven workspace or trusted temporary allocation and requires approval.", "破坏性操作的目标 %s 不在已验证的工作区或可信临时分配中，需要批准。", "Das destruktive Ziel %s liegt außerhalb des nachgewiesenen Arbeitsbereichs oder einer vertrauenswürdigen temporären Zuweisung und erfordert eine Genehmigung.", "破壊的操作の対象 %s は検証済み workspace または信頼済み一時割り当ての外にあるため、承認が必要です。", "파괴적 작업 대상 %s이(가) 검증된 작업 공간 또는 신뢰된 임시 할당 밖에 있어 승인이 필요합니다.", "Цель разрушительной операции %s находится вне подтверждённой рабочей области или доверенного временного размещения и требует подтверждения.")
	add(KeyShellPolicyAskDestructive,
		"The destructive operation requires explicit approval: %s", "此破坏性操作需要明确批准：%s", "Die destruktive Operation erfordert eine ausdrückliche Genehmigung: %s", "この破壊的操作には明示的な承認が必要です: %s", "이 파괴적 작업에는 명시적 승인이 필요합니다: %s", "Разрушительная операция требует явного подтверждения: %s")
	add(KeyShellPolicyAskStructural,
		"The command structure requires explicit approval: %s", "此命令结构需要明确批准：%s", "Die Befehlsstruktur erfordert eine ausdrückliche Genehmigung: %s", "このコマンド構造には明示的な承認が必要です: %s", "이 명령 구조에는 명시적 승인이 필요합니다: %s", "Структура команды требует явного подтверждения: %s")
	add(KeyShellPolicyAskUnrestrictedCode,
		"Execution of %s is not completely modeled and requires explicit approval for this invocation.", "无法完整建模 %s 的执行行为；本次调用需要明确批准。", "Die Ausführung von %s ist nicht vollständig modelliert und erfordert für diesen Aufruf eine ausdrückliche Genehmigung.", "%s の実行動作を完全にはモデル化できないため、この呼び出しには明示的な承認が必要です。", "%s 실행 동작이 완전히 모델링되지 않았으므로 이번 호출에는 명시적 승인이 필요합니다.", "Выполнение %s смоделировано не полностью; для этого вызова требуется явное подтверждение.")
	add(KeyShellPolicyRemediationApprove,
		"Run the command through an interactive approval prompt or replace the dynamic target with a verified path.", "请通过交互式批准提示运行命令，或将动态目标替换为已验证路径。", "Führe den Befehl über eine interaktive Genehmigungsabfrage aus oder ersetze das dynamische Ziel durch einen überprüften Pfad.", "対話式の承認を経てコマンドを実行するか、動的な対象を検証済みのパスに置き換えてください。", "대화형 승인 절차로 명령을 실행하거나 동적 대상을 검증된 경로로 바꾸세요.", "Выполните команду через интерактивный запрос подтверждения или замените динамическую цель проверенным путём.")
}
