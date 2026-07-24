package i18n

// Semantic copy for first-party tool permission and Bash safety decisions.
// Tool names, commands, paths, permission rule names, and matched shell text
// are supplied as format values and remain untranslated.
const (
	KeyToolPermissionInvalidPath         Key = "tool.permission.path.invalid"
	KeyToolPermissionReadDenied          Key = "tool.permission.read.denied"
	KeyToolPermissionReadRequired        Key = "tool.permission.read.required"
	KeyToolPermissionReadUNC             Key = "tool.permission.read.unc"
	KeyToolPermissionReadBinary          Key = "tool.permission.read.binary"
	KeyToolPermissionReadDevice          Key = "tool.permission.read.device"
	KeyToolPermissionWritePlanMode       Key = "tool.permission.write.plan_mode"
	KeyToolPermissionEditPlanMode        Key = "tool.permission.edit.plan_mode"
	KeyToolPermissionNotebookPlanMode    Key = "tool.permission.notebook.plan_mode"
	KeyToolPermissionModifyRequired      Key = "tool.permission.modify.required"
	KeyToolPermissionOutsideDirectories  Key = "tool.permission.path.outside_directories"
	KeyToolPermissionWriteDenied         Key = "tool.permission.write.denied"
	KeyToolPermissionWriteProtected      Key = "tool.permission.write.protected"
	KeyToolPermissionWritePending        Key = "tool.permission.write.pending"
	KeyToolPermissionSearchDenied        Key = "tool.permission.search.denied"
	KeyToolPermissionSearchRequired      Key = "tool.permission.search.required"
	KeyToolPermissionWebFetchDenied      Key = "tool.permission.web_fetch.denied"
	KeyToolPermissionWebFetchPending     Key = "tool.permission.web_fetch.pending"
	KeyToolPermissionWebDomainBlocked    Key = "tool.permission.web.domain_blocked"
	KeyToolPermissionWebDomainNotAllowed Key = "tool.permission.web.domain_not_allowed"
	KeyToolPermissionWebInvalidURL       Key = "tool.permission.web.invalid_url"
	KeyToolPermissionWebSearchRequired   Key = "tool.permission.web_search.required"
	KeyToolPermissionExitPlanNotActive   Key = "tool.permission.exit_plan.not_active"
	KeyToolPermissionExitPlanConfirm     Key = "tool.permission.exit_plan.confirm"
	KeyToolPermissionAgentSpawn          Key = "tool.permission.agent.spawn"

	KeyToolPermissionBashPlanMode            Key = "tool.permission.bash.plan_mode"
	KeyToolPermissionBashRuleDenied          Key = "tool.permission.bash.rule_denied"
	KeyToolPermissionBashRuleApproval        Key = "tool.permission.bash.rule_approval"
	KeyToolPermissionBashGenericApproval     Key = "tool.permission.bash.generic_approval"
	KeyToolPermissionBashDestructiveApproval Key = "tool.permission.bash.destructive_approval"
	KeyToolPermissionBashProcessSubstitution Key = "tool.permission.bash.process_substitution"
	KeyToolPermissionBashMultipleDirectories Key = "tool.permission.bash.multiple_directories"
	KeyToolPermissionBashCDAndGit            Key = "tool.permission.bash.cd_and_git"
	KeyToolPermissionBashCDAndRedirect       Key = "tool.permission.bash.cd_and_redirect"
	KeyToolPermissionBashBareGit             Key = "tool.permission.bash.bare_git"
	KeyToolPermissionBashGitInternal         Key = "tool.permission.bash.git_internal"

	KeyBashSecurityRemotePipeShell        Key = "bash.security.remote_pipe_shell"
	KeyBashSecurityEncodedPipeShell       Key = "bash.security.encoded_pipe_shell"
	KeyBashSecurityEncodedSubstitution    Key = "bash.security.encoded_substitution"
	KeyBashSecurityDynamicEval            Key = "bash.security.dynamic_eval"
	KeyBashSecurityReverseShell           Key = "bash.security.reverse_shell"
	KeyBashSecurityScriptPayload          Key = "bash.security.script_payload"
	KeyBashSecurityHistoryTampering       Key = "bash.security.history_tampering"
	KeyBashSecurityObfuscatedPayload      Key = "bash.security.obfuscated_payload"
	KeyBashSecuritySSHInlineShell         Key = "bash.security.ssh_inline_shell"
	KeyBashSecurityRecursiveDelete        Key = "bash.security.recursive_delete"
	KeyBashSecurityForkBomb               Key = "bash.security.fork_bomb"
	KeyBashSecurityChmodSetuid            Key = "bash.security.chmod_setuid"
	KeyBashSecurityChmodWorldWritable     Key = "bash.security.chmod_world_writable"
	KeyBashSecurityDownloadSubstitution   Key = "bash.security.download_substitution"
	KeyBashSecurityRawDiskWrite           Key = "bash.security.raw_disk_write"
	KeyBashSecurityFilesystemFormat       Key = "bash.security.filesystem_format"
	KeyBashSecurityPowerOperation         Key = "bash.security.power_operation"
	KeyBashSecurityCrontabRemoval         Key = "bash.security.crontab_removal"
	KeyBashSecurityFirewallFlush          Key = "bash.security.firewall_flush"
	KeyBashSecurityPermissionLockout      Key = "bash.security.permission_lockout"
	KeyBashSecurityCriticalService        Key = "bash.security.critical_service"
	KeyBashSecurityPrivilegedUserDelete   Key = "bash.security.privileged_user_delete"
	KeyBashSecurityDiskRepartition        Key = "bash.security.disk_repartition"
	KeyBashSecurityCompoundMultipleCD     Key = "bash.security.compound_multiple_cd"
	KeyBashSecurityCompoundCrossPipeWrite Key = "bash.security.compound_cross_pipe_write"
)

func init() {
	entries := map[Key][6]string{
		KeyToolPermissionInvalidPath: {
			"The path is invalid.", "路径无效。", "Der Pfad ist ungültig.", "パスが無効です。", "경로가 올바르지 않습니다.", "Недопустимый путь."},
		KeyToolPermissionReadDenied: {
			"This file is in a directory denied by your permission settings.", "此文件位于权限设置禁止访问的目录中。", "Diese Datei liegt in einem durch deine Berechtigungseinstellungen gesperrten Verzeichnis.", "このファイルは権限設定で拒否されたディレクトリにあります。", "이 파일은 권한 설정에서 거부된 디렉터리에 있습니다.", "Файл находится в каталоге, запрещённом настройками разрешений."},
		KeyToolPermissionReadRequired: {
			"Permission is required to read %s.", "读取 %s 需要授权。", "Zum Lesen von %s ist eine Berechtigung erforderlich.", "%s を読み取るには許可が必要です。", "%s을(를) 읽으려면 권한이 필요합니다.", "Для чтения %s требуется разрешение."},
		KeyToolPermissionReadUNC: {
			"Permission is required to read %s because this UNC path may access network resources.", "读取 %s 需要授权，因为此 UNC 路径可能访问网络资源。", "Zum Lesen von %s ist eine Berechtigung erforderlich, da dieser UNC-Pfad auf Netzwerkressourcen zugreifen kann.", "%s はネットワークリソースにアクセスする可能性がある UNC パスのため、読み取りには許可が必要です。", "%s은(는) 네트워크 리소스에 접근할 수 있는 UNC 경로이므로 읽기 권한이 필요합니다.", "Для чтения %s требуется разрешение: этот UNC-путь может обращаться к сетевым ресурсам."},
		KeyToolPermissionReadBinary: {
			"This tool cannot read binary %s files. Use a tool intended for binary file analysis.", "此工具无法读取二进制 %s 文件。请使用适合分析二进制文件的工具。", "Dieses Tool kann binäre %s-Dateien nicht lesen. Verwende ein Tool für die Analyse von Binärdateien.", "このツールではバイナリ %s ファイルを読み取れません。バイナリ解析用のツールを使用してください。", "이 도구는 바이너리 %s 파일을 읽을 수 없습니다. 바이너리 파일 분석용 도구를 사용하세요.", "Этот инструмент не читает двоичные файлы %s. Используйте инструмент для анализа двоичных файлов."},
		KeyToolPermissionReadDevice: {
			"Cannot read %s because this device file may block or produce infinite output.", "%s 是设备文件，可能导致阻塞或产生无限输出，因此无法读取。", "%s kann nicht gelesen werden, weil diese Gerätedatei blockieren oder endlose Ausgaben erzeugen kann.", "%s はブロックまたは無限出力を引き起こす可能性があるデバイスファイルのため、読み取れません。", "%s은(는) 차단되거나 무한 출력을 생성할 수 있는 장치 파일이므로 읽을 수 없습니다.", "Нельзя прочитать %s: файл устройства может заблокировать операцию или выдавать бесконечный поток данных."},
		KeyToolPermissionWritePlanMode: {
			"Cannot write files in plan mode.", "计划模式下无法写入文件。", "Im Planungsmodus können keine Dateien geschrieben werden.", "plan mode ではファイルを書き込めません。", "plan mode에서는 파일을 쓸 수 없습니다.", "В режиме планирования запись файлов недоступна."},
		KeyToolPermissionEditPlanMode: {
			"Cannot edit files in plan mode.", "计划模式下无法编辑文件。", "Im Planungsmodus können keine Dateien bearbeitet werden.", "plan mode ではファイルを編集できません。", "plan mode에서는 파일을 편집할 수 없습니다.", "В режиме планирования редактирование файлов недоступно."},
		KeyToolPermissionNotebookPlanMode: {
			"Cannot edit notebooks in plan mode.", "计划模式下无法编辑 notebook。", "Im Planungsmodus können keine Notebooks bearbeitet werden.", "plan mode では notebook を編集できません。", "plan mode에서는 notebook을 편집할 수 없습니다.", "В режиме планирования редактирование notebook недоступно."},
		KeyToolPermissionModifyRequired: {
			"%s requires permission to modify %s.", "%s 修改 %s 需要授权。", "%s benötigt eine Berechtigung, um %s zu ändern.", "%s で %s を変更するには許可が必要です。", "%s에서 %s을(를) 수정하려면 권한이 필요합니다.", "%s требуется разрешение на изменение %s."},
		KeyToolPermissionOutsideDirectories: {
			"The path is outside the allowed working directories: %s", "路径不在允许的工作目录内：%s", "Der Pfad liegt außerhalb der zulässigen Arbeitsverzeichnisse: %s", "パスは許可された作業ディレクトリの外にあります: %s", "경로가 허용된 작업 디렉터리 밖에 있습니다: %s", "Путь находится вне разрешённых рабочих каталогов: %s"},
		KeyToolPermissionWriteDenied: {
			"Permission to edit %s has been denied.", "编辑 %s 的权限已被拒绝。", "Die Berechtigung zum Bearbeiten von %s wurde verweigert.", "%s を編集する権限が拒否されました。", "%s 편집 권한이 거부되었습니다.", "В разрешении на редактирование %s отказано."},
		KeyToolPermissionWriteProtected: {
			"Permission is required to write to the protected path %s.", "写入受保护路径 %s 需要授权。", "Zum Schreiben in den geschützten Pfad %s ist eine Berechtigung erforderlich.", "保護されたパス %s への書き込みには許可が必要です。", "보호된 경로 %s에 쓰려면 권한이 필요합니다.", "Для записи в защищённый путь %s требуется разрешение."},
		KeyToolPermissionWritePending: {
			"Permission is required to write to %s.", "写入 %s 需要授权。", "Zum Schreiben nach %s ist eine Berechtigung erforderlich.", "%s への書き込みには許可が必要です。", "%s에 쓰려면 권한이 필요합니다.", "Для записи в %s требуется разрешение."},
		KeyToolPermissionSearchDenied: {
			"Permission to use %s with pattern %s has been denied.", "使用 %s 搜索模式 %s 的权限已被拒绝。", "Die Berechtigung für %s mit dem Muster %s wurde verweigert.", "%s でパターン %s を使用する権限が拒否されました。", "%s에서 패턴 %s을(를) 사용할 권한이 거부되었습니다.", "В разрешении на использование %s с шаблоном %s отказано."},
		KeyToolPermissionSearchRequired: {
			"Permission is required to use %s with pattern %s.", "使用 %s 搜索模式 %s 需要授权。", "Für %s mit dem Muster %s ist eine Berechtigung erforderlich.", "%s でパターン %s を使用するには許可が必要です。", "%s에서 패턴 %s을(를) 사용하려면 권한이 필요합니다.", "Для использования %s с шаблоном %s требуется разрешение."},
		KeyToolPermissionWebFetchDenied: {
			"WebFetch access to %s has been denied.", "WebFetch 对 %s 的访问已被拒绝。", "Der WebFetch-Zugriff auf %s wurde verweigert.", "WebFetch による %s へのアクセスが拒否されました。", "%s에 대한 WebFetch 접근이 거부되었습니다.", "Доступ WebFetch к %s запрещён."},
		KeyToolPermissionWebFetchPending: {
			"WebFetch requires permission before it can access this resource.", "WebFetch 需要获得授权才能访问此资源。", "WebFetch benötigt eine Berechtigung, bevor auf diese Ressource zugegriffen werden kann.", "このリソースにアクセスするには WebFetch の許可が必要です。", "WebFetch가 이 리소스에 접근하려면 권한이 필요합니다.", "WebFetch требуется разрешение для доступа к этому ресурсу."},
		KeyToolPermissionWebDomainBlocked: {
			"Domain %q is blocked by policy.", "域名 %q 已被策略禁止。", "Die Domain %q ist durch eine Richtlinie gesperrt.", "ドメイン %q はポリシーでブロックされています。", "도메인 %q은(는) 정책에 의해 차단되었습니다.", "Домен %q заблокирован политикой."},
		KeyToolPermissionWebDomainNotAllowed: {
			"Domain %q is not in the allowed list.", "域名 %q 不在允许列表中。", "Die Domain %q steht nicht auf der Zulassungsliste.", "ドメイン %q は許可リストにありません。", "도메인 %q은(는) 허용 목록에 없습니다.", "Домена %q нет в списке разрешённых."},
		KeyToolPermissionWebInvalidURL: {
			"The URL is invalid or has no host.", "URL 无效或缺少 host。", "Die URL ist ungültig oder enthält keinen Host.", "URL が無効か、host がありません。", "URL이 올바르지 않거나 host가 없습니다.", "URL недействителен или не содержит host."},
		KeyToolPermissionWebSearchRequired: {
			"WebSearch requires permission.", "WebSearch 需要授权。", "WebSearch benötigt eine Berechtigung.", "WebSearch には許可が必要です。", "WebSearch에는 권한이 필요합니다.", "WebSearch требуется разрешение."},
		KeyToolPermissionExitPlanNotActive: {
			"You are not in plan mode. ExitPlanMode can only be used after writing a plan.", "当前不在计划模式中。只有编写计划后才能使用 ExitPlanMode。", "Du bist nicht im Planungsmodus. ExitPlanMode kann erst nach dem Schreiben eines Plans verwendet werden.", "plan mode ではありません。ExitPlanMode は計画を作成した後にのみ使用できます。", "plan mode가 아닙니다. ExitPlanMode는 계획을 작성한 후에만 사용할 수 있습니다.", "Сейчас не активен режим планирования. ExitPlanMode можно использовать только после подготовки плана."},
		KeyToolPermissionExitPlanConfirm: {
			"Exit plan mode?", "退出计划模式？", "Planungsmodus verlassen?", "plan mode を終了しますか？", "plan mode를 종료할까요?", "Выйти из режима планирования?"},
		KeyToolPermissionAgentSpawn: {
			"Agent requires permission to start sub-agents.", "Agent 启动 sub-agent 需要授权。", "Agent benötigt eine Berechtigung, um Sub-Agents zu starten.", "Agent が sub-agent を開始するには許可が必要です。", "Agent가 sub-agent를 시작하려면 권한이 필요합니다.", "Agent требуется разрешение на запуск sub-agent."},

		KeyToolPermissionBashPlanMode: {
			"Cannot use Bash in plan mode; exit plan mode first.", "计划模式下无法使用 Bash；请先退出计划模式。", "Bash kann im Planungsmodus nicht verwendet werden; verlasse zuerst den Planungsmodus.", "plan mode では Bash を使用できません。先に plan mode を終了してください。", "plan mode에서는 Bash를 사용할 수 없습니다. 먼저 plan mode를 종료하세요.", "Bash нельзя использовать в режиме планирования; сначала выйдите из него."},
		KeyToolPermissionBashRuleDenied: {
			"The command was denied by a permission rule.", "此命令已被权限规则拒绝。", "Der Befehl wurde durch eine Berechtigungsregel verweigert.", "コマンドは権限ルールによって拒否されました。", "명령이 권한 규칙에 의해 거부되었습니다.", "Команда запрещена правилом разрешений."},
		KeyToolPermissionBashRuleApproval: {
			"The command requires user approval.", "此命令需要用户批准。", "Der Befehl erfordert die Zustimmung des Benutzers.", "コマンドにはユーザーの承認が必要です。", "명령에 사용자 승인이 필요합니다.", "Команда требует подтверждения пользователя."},
		KeyToolPermissionBashGenericApproval: {
			"The Bash command requires permission.", "此 Bash 命令需要授权。", "Der Bash-Befehl benötigt eine Berechtigung.", "Bash コマンドには許可が必要です。", "Bash 명령에 권한이 필요합니다.", "Для команды Bash требуется разрешение."},
		KeyToolPermissionBashDestructiveApproval: {
			"This destructive command requires explicit approval.", "此破坏性命令需要明确批准。", "Dieser destruktive Befehl erfordert eine ausdrückliche Zustimmung.", "この破壊的なコマンドには明示的な承認が必要です。", "이 파괴적인 명령에는 명시적인 승인이 필요합니다.", "Эта разрушительная команда требует явного подтверждения."},
		KeyToolPermissionBashProcessSubstitution: {
			"Process substitution (>(...) or <(...)) can execute arbitrary commands and requires manual approval.", "进程替换（>(...) 或 <(...)）可执行任意命令，需要手动批准。", "Prozesssubstitution (>(...) oder <(...)) kann beliebige Befehle ausführen und erfordert eine manuelle Zustimmung.", "プロセス置換 (>(...) または <(...)) は任意のコマンドを実行できるため、手動承認が必要です。", "프로세스 대체(>(...) 또는 <(...))는 임의 명령을 실행할 수 있으므로 수동 승인이 필요합니다.", "Подстановка процесса (>(...) или <(...)) может выполнить произвольные команды и требует ручного подтверждения."},
		KeyToolPermissionBashMultipleDirectories: {
			"Multiple directory changes in one command require approval for clarity.", "单条命令中多次切换目录需要批准，以确保路径清晰。", "Mehrere Verzeichniswechsel in einem Befehl erfordern zur Eindeutigkeit eine Zustimmung.", "1 つのコマンドで複数回ディレクトリを変更する場合は、パスを明確にするため承認が必要です。", "한 명령에서 디렉터리를 여러 번 변경하면 경로 확인을 위해 승인이 필요합니다.", "Несколько смен каталогов в одной команде требуют подтверждения для однозначности путей."},
		KeyToolPermissionBashCDAndGit: {
			"Compound commands with cd and git require approval to prevent bare repository attacks.", "同时包含 cd 和 git 的复合命令需要批准，以防止针对 bare repository 的攻击。", "Zusammengesetzte Befehle mit cd und git erfordern eine Zustimmung, um Angriffe über Bare-Repositories zu verhindern.", "cd と git を含む複合コマンドは、bare repository 攻撃を防ぐため承認が必要です。", "cd와 git이 포함된 복합 명령은 bare repository 공격을 방지하기 위해 승인이 필요합니다.", "Составные команды с cd и git требуют подтверждения для защиты от атак через bare repository."},
		KeyToolPermissionBashCDAndRedirect: {
			"Commands that change directories and write via output redirection require explicit approval so paths are evaluated correctly.", "切换目录并通过输出重定向写入的命令需要明确批准，以确保正确判断路径。", "Befehle, die das Verzeichnis wechseln und per Ausgabeumleitung schreiben, erfordern eine ausdrückliche Zustimmung, damit Pfade korrekt ausgewertet werden.", "ディレクトリを変更して出力リダイレクトで書き込むコマンドは、パスを正しく評価するため明示的な承認が必要です。", "디렉터리를 변경하고 출력 리디렉션으로 쓰는 명령은 경로를 올바르게 평가하기 위해 명시적인 승인이 필요합니다.", "Команды, которые меняют каталог и записывают через перенаправление вывода, требуют явного подтверждения для корректной оценки путей."},
		KeyToolPermissionBashBareGit: {
			"Git commands in directories with a bare repository structure require enhanced permission checks.", "在具有 bare repository 结构的目录中运行 Git 命令需要更严格的权限检查。", "Git-Befehle in Verzeichnissen mit Bare-Repository-Struktur erfordern erweiterte Berechtigungsprüfungen.", "bare repository 構造のディレクトリで Git コマンドを実行する場合は、強化された権限チェックが必要です。", "bare repository 구조의 디렉터리에서 Git 명령을 실행하면 강화된 권한 검사가 필요합니다.", "Команды Git в каталогах со структурой bare repository требуют усиленной проверки разрешений."},
		KeyToolPermissionBashGitInternal: {
			"Compound commands that create git internal files and run git require enhanced permission checks.", "创建 git 内部文件并运行 git 的复合命令需要更严格的权限检查。", "Zusammengesetzte Befehle, die interne git-Dateien erstellen und git ausführen, erfordern erweiterte Berechtigungsprüfungen.", "git の内部ファイルを作成して git を実行する複合コマンドには、強化された権限チェックが必要です。", "git 내부 파일을 만들고 git을 실행하는 복합 명령에는 강화된 권한 검사가 필요합니다.", "Составные команды, создающие внутренние файлы git и запускающие git, требуют усиленной проверки разрешений."},

		KeyBashSecurityRemotePipeShell: {
			"Downloaded content is piped directly to a shell.", "下载的内容被直接通过 pipe 传给 shell。", "Heruntergeladene Inhalte werden direkt an eine Shell weitergeleitet.", "ダウンロードした内容が shell に直接 pipe されています。", "다운로드한 콘텐츠가 shell로 직접 pipe됩니다.", "Загруженные данные напрямую передаются в shell."},
		KeyBashSecurityEncodedPipeShell: {
			"An encoded payload is decoded and piped to a shell.", "编码 payload 被解码后通过 pipe 传给 shell。", "Eine codierte Payload wird decodiert und an eine Shell weitergeleitet.", "エンコードされた payload がデコードされ、shell に pipe されています。", "인코딩된 payload가 디코딩되어 shell로 pipe됩니다.", "Закодированная payload декодируется и передаётся в shell."},
		KeyBashSecurityEncodedSubstitution: {
			"A decoded payload is executed inside command substitution.", "解码后的 payload 在命令替换中执行。", "Eine decodierte Payload wird innerhalb einer Befehlssubstitution ausgeführt.", "デコードされた payload がコマンド置換内で実行されます。", "디코딩된 payload가 명령 대체 안에서 실행됩니다.", "Декодированная payload выполняется внутри подстановки команды."},
		KeyBashSecurityDynamicEval: {
			"eval executes a dynamically constructed or decoded command.", "eval 会执行动态构造或解码后的命令。", "eval führt einen dynamisch erzeugten oder decodierten Befehl aus.", "eval が動的に構築またはデコードされたコマンドを実行します。", "eval이 동적으로 구성되거나 디코딩된 명령을 실행합니다.", "eval выполняет динамически созданную или декодированную команду."},
		KeyBashSecurityReverseShell: {
			"The command may open a reverse shell through /dev/tcp or /dev/udp.", "此命令可能通过 /dev/tcp 或 /dev/udp 打开 reverse shell。", "Der Befehl kann über /dev/tcp oder /dev/udp eine Reverse Shell öffnen.", "コマンドが /dev/tcp または /dev/udp を通じて reverse shell を開く可能性があります。", "명령이 /dev/tcp 또는 /dev/udp를 통해 reverse shell을 열 수 있습니다.", "Команда может открыть reverse shell через /dev/tcp или /dev/udp."},
		KeyBashSecurityScriptPayload: {
			"A %s one-liner starts a shell or executes a system command.", "%s one-liner 会启动 shell 或执行系统命令。", "Ein %s-One-Liner startet eine Shell oder führt einen Systembefehl aus.", "%s one-liner が shell を起動またはシステムコマンドを実行します。", "%s one-liner가 shell을 시작하거나 시스템 명령을 실행합니다.", "%s one-liner запускает shell или выполняет системную команду."},
		KeyBashSecurityHistoryTampering: {
			"%s disables or clears shell history.", "%s 会禁用或清除 shell history。", "%s deaktiviert oder löscht den Shell-Verlauf.", "%s が shell history を無効化または消去します。", "%s이(가) shell history를 비활성화하거나 지웁니다.", "%s отключает или очищает историю shell."},
		KeyBashSecurityObfuscatedPayload: {
			"The command contains an obfuscated hexadecimal payload.", "命令包含经过混淆的十六进制 payload。", "Der Befehl enthält eine verschleierte hexadezimale Payload.", "コマンドに難読化された 16 進 payload が含まれています。", "명령에 난독화된 16진수 payload가 포함되어 있습니다.", "Команда содержит обфусцированную шестнадцатеричную payload."},
		KeyBashSecuritySSHInlineShell: {
			"ssh runs an inline shell command on a remote host.", "ssh 会在远程 host 上运行内联 shell 命令。", "ssh führt auf einem entfernten Host einen Inline-Shell-Befehl aus.", "ssh がリモート host でインライン shell コマンドを実行します。", "ssh가 원격 host에서 인라인 shell 명령을 실행합니다.", "ssh выполняет встроенную shell-команду на удалённом host."},
		KeyBashSecurityRecursiveDelete: {
			"The command recursively deletes %s.", "此命令会递归删除 %s。", "Der Befehl löscht %s rekursiv.", "コマンドが %s を再帰的に削除します。", "명령이 %s을(를) 재귀적으로 삭제합니다.", "Команда рекурсивно удаляет %s."},
		KeyBashSecurityForkBomb: {
			"The command contains a fork bomb.", "命令包含 fork bomb。", "Der Befehl enthält eine Forkbomb.", "コマンドに fork bomb が含まれています。", "명령에 fork bomb가 포함되어 있습니다.", "Команда содержит fork bomb."},
		KeyBashSecurityChmodSetuid: {
			"chmod sets the setuid bit on a system path.", "chmod 会为系统路径设置 setuid bit。", "chmod setzt das setuid-Bit auf einem Systempfad.", "chmod がシステムパスに setuid bit を設定します。", "chmod가 시스템 경로에 setuid bit를 설정합니다.", "chmod устанавливает setuid bit для системного пути."},
		KeyBashSecurityChmodWorldWritable: {
			"chmod makes a system path world-writable.", "chmod 会使系统路径对所有用户可写。", "chmod macht einen Systempfad für alle beschreibbar.", "chmod がシステムパスを全ユーザー書き込み可能にします。", "chmod가 시스템 경로를 모든 사용자가 쓸 수 있게 만듭니다.", "chmod делает системный путь доступным для записи всем пользователям."},
		KeyBashSecurityDownloadSubstitution: {
			"Command substitution downloads and executes remote content.", "命令替换会下载并执行远程内容。", "Eine Befehlssubstitution lädt entfernte Inhalte herunter und führt sie aus.", "コマンド置換がリモート内容をダウンロードして実行します。", "명령 대체가 원격 콘텐츠를 다운로드하고 실행합니다.", "Подстановка команды загружает и выполняет удалённые данные."},
		KeyBashSecurityRawDiskWrite: {
			"The command writes raw bytes to a block device and may cause irreversible data loss.", "此命令会向 block device 写入原始字节，可能造成不可恢复的数据丢失。", "Der Befehl schreibt Rohdaten auf ein Blockgerät und kann unwiederbringlichen Datenverlust verursachen.", "コマンドが block device に生データを書き込み、回復不能なデータ損失を引き起こす可能性があります。", "명령이 block device에 원시 바이트를 써서 복구할 수 없는 데이터 손실을 일으킬 수 있습니다.", "Команда записывает необработанные данные на block device и может привести к необратимой потере данных."},
		KeyBashSecurityFilesystemFormat: {
			"The command formats a filesystem and irreversibly erases its contents.", "此命令会格式化 filesystem，并不可恢复地清除其中的内容。", "Der Befehl formatiert ein Dateisystem und löscht dessen Inhalt unwiederbringlich.", "コマンドが filesystem をフォーマットし、内容を回復不能な形で消去します。", "명령이 filesystem을 포맷하고 내용을 복구할 수 없게 지웁니다.", "Команда форматирует filesystem и необратимо стирает его содержимое."},
		KeyBashSecurityPowerOperation: {
			"%s changes the system power state.", "%s 会改变系统电源状态。", "%s ändert den Energiezustand des Systems.", "%s がシステムの電源状態を変更します。", "%s이(가) 시스템 전원 상태를 변경합니다.", "%s изменяет состояние питания системы."},
		KeyBashSecurityCrontabRemoval: {
			"crontab -r removes the user's crontab.", "crontab -r 会删除用户的 crontab。", "crontab -r entfernt die Crontab des Benutzers.", "crontab -r がユーザーの crontab を削除します。", "crontab -r이 사용자의 crontab을 삭제합니다.", "crontab -r удаляет crontab пользователя."},
		KeyBashSecurityFirewallFlush: {
			"%s flushes firewall rules.", "%s 会清空 firewall 规则。", "%s löscht die Firewall-Regeln.", "%s が firewall ルールを消去します。", "%s이(가) firewall 규칙을 비웁니다.", "%s очищает правила firewall."},
		KeyBashSecurityPermissionLockout: {
			"chmod removes permissions from a system path and may cause a lockout.", "chmod 会移除系统路径的权限，可能导致无法访问。", "chmod entfernt Berechtigungen von einem Systempfad und kann den Zugriff sperren.", "chmod がシステムパスの権限を削除し、アクセス不能を引き起こす可能性があります。", "chmod가 시스템 경로의 권한을 제거하여 접근 불가 상태를 일으킬 수 있습니다.", "chmod снимает разрешения с системного пути и может заблокировать доступ."},
		KeyBashSecurityCriticalService: {
			"%s stops or disables a critical service and may lock out the operator.", "%s 会停止或禁用关键服务，可能导致操作者无法访问系统。", "%s stoppt oder deaktiviert einen kritischen Dienst und kann den Bediener aussperren.", "%s が重要なサービスを停止または無効化し、操作者がアクセスできなくなる可能性があります。", "%s이(가) 중요 서비스를 중지하거나 비활성화하여 운영자의 접근을 막을 수 있습니다.", "%s останавливает или отключает критически важную службу и может заблокировать доступ оператора."},
		KeyBashSecurityPrivilegedUserDelete: {
			"userdel removes a privileged user account.", "userdel 会删除特权用户账户。", "userdel entfernt ein privilegiertes Benutzerkonto.", "userdel が特権ユーザーアカウントを削除します。", "userdel이 권한 있는 사용자 계정을 삭제합니다.", "userdel удаляет привилегированную учётную запись."},
		KeyBashSecurityDiskRepartition: {
			"fdisk can repartition a physical disk.", "fdisk 可能会重新分区物理磁盘。", "fdisk kann einen physischen Datenträger neu partitionieren.", "fdisk が物理ディスクを再パーティションする可能性があります。", "fdisk가 물리 디스크를 다시 파티션할 수 있습니다.", "fdisk может переразметить физический диск."},
		KeyBashSecurityCompoundMultipleCD: {
			"A compound expression contains more than one cd command.", "复合表达式中包含多个 cd 命令。", "Ein zusammengesetzter Ausdruck enthält mehr als einen cd-Befehl.", "複合式に複数の cd コマンドが含まれています。", "복합 표현식에 cd 명령이 두 개 이상 포함되어 있습니다.", "Составное выражение содержит несколько команд cd."},
		KeyBashSecurityCompoundCrossPipeWrite: {
			"A write follows cd across a pipe, where the working directory does not propagate.", "写入操作通过 pipe 紧跟在 cd 之后，但工作目录不会跨 pipe 传递。", "Ein Schreibvorgang folgt cd über eine Pipe, obwohl das Arbeitsverzeichnis dort nicht weitergegeben wird.", "作業ディレクトリが引き継がれない pipe をまたいで、cd の後に書き込みがあります。", "작업 디렉터리가 전달되지 않는 pipe를 사이에 두고 cd 다음에 쓰기 작업이 있습니다.", "Операция записи следует за cd через pipe, где рабочий каталог не наследуется."},
	}
	for key, values := range entries {
		semanticTranslations[key] = map[Language]string{
			LangEN: values[0], LangZH: values[1], LangDE: values[2],
			LangJA: values[3], LangKO: values[4], LangRU: values[5],
		}
	}
}
