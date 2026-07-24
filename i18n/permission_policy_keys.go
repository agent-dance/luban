package i18n

const (
	KeyPermissionModeAllowAllFrozen       Key = "permission.mode.allow_all_frozen"
	KeyPermissionDenyDisallowedTool       Key = "permission.deny.disallowed_tool"
	KeyPermissionDenyNotAllowedTool       Key = "permission.deny.not_allowed_tool"
	KeyPermissionDenySnapshotRule         Key = "permission.deny.snapshot_rule"
	KeyPermissionTestingApprovalRequired  Key = "permission.testing.approval_required"
	KeyPermissionTestingPolicy            Key = "permission.testing.policy"
	KeyPermissionDenyTestingPrompt        Key = "permission.deny.testing_prompt"
	KeyPermissionDenySnapshotAsk          Key = "permission.deny.snapshot_ask"
	KeyPermissionAskAlwaysPolicy          Key = "permission.policy.ask_always"
	KeyPermissionDenyAskAlways            Key = "permission.deny.ask_always"
	KeyPermissionDenyRule                 Key = "permission.deny.rule"
	KeyPermissionRuleFallback             Key = "permission.policy.rule_fallback"
	KeyPermissionAdvisoryPolicy           Key = "permission.policy.advisory"
	KeyPermissionMandatoryPolicy          Key = "permission.policy.mandatory"
	KeyPermissionApprovalRequired         Key = "permission.deny.approval_required"
	KeyPermissionSnapshotSource           Key = "permission.policy.snapshot"
	KeyPermissionConfiguredRule           Key = "permission.policy.configured_rule"
	KeyPermissionConfiguredPatternRule    Key = "permission.policy.configured_pattern_rule"
	KeyPermissionSafetyProtectedPath      Key = "permission.safety.protected_path"
	KeyPermissionSafetyUnavailable        Key = "permission.safety.unavailable"
	KeyPermissionSafetyDangerousCommand   Key = "permission.safety.dangerous_command"
	KeyPermissionSafetyShellProtectedPath Key = "permission.safety.shell_protected_path"
	KeyPermissionSafetyPowerShell         Key = "permission.safety.powershell"
	KeyPermissionPreviewSendMessage       Key = "permission.preview.send_message"
	KeyPermissionPreviewSendTarget        Key = "permission.preview.send_target"
	KeyPermissionEnvironmentRoot          Key = "permission.environment.root"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyPermissionModeAllowAllFrozen,
		"Cannot enable Allow All mode after the session has started", "session 启动后无法启用全部允许模式", "Der Modus „Alles erlauben“ kann nach dem Sitzungsstart nicht aktiviert werden", "セッション開始後は「すべて許可」モードを有効にできません", "세션이 시작된 후에는 모두 허용 모드를 활성화할 수 없습니다", "После запуска сеанса нельзя включить режим «Разрешить всё»")
	add(KeyPermissionDenyDisallowedTool,
		"The tool is in the denied list", "该工具位于禁止列表中", "Das Tool steht auf der Sperrliste", "ツールは拒否リストに含まれています", "도구가 거부 목록에 있습니다", "Инструмент находится в списке запрещённых")
	add(KeyPermissionDenyNotAllowedTool,
		"The tool is not in the allowed list", "该工具不在允许列表中", "Das Tool steht nicht auf der Zulassungsliste", "ツールは許可リストに含まれていません", "도구가 허용 목록에 없습니다", "Инструмента нет в списке разрешённых")
	add(KeyPermissionDenySnapshotRule,
		"Denied by the sub-agent's spawn-time permission snapshot", "已被 sub-agent 启动时的权限快照拒绝", "Durch den Berechtigungs-Snapshot des Sub-Agents beim Start abgelehnt", "sub-agent 起動時の権限スナップショットにより拒否されました", "sub-agent 시작 시점 권한 스냅샷에 의해 거부되었습니다", "Отклонено снимком разрешений sub-agent на момент запуска")
	add(KeyPermissionTestingApprovalRequired,
		"TestingPermission requires interactive approval", "TestingPermission 需要交互式批准", "TestingPermission erfordert eine interaktive Genehmigung", "TestingPermission には対話形式の承認が必要です", "TestingPermission에는 대화형 승인이 필요합니다", "TestingPermission требует интерактивного подтверждения")
	add(KeyPermissionTestingPolicy,
		"TestingPermission interactive policy", "TestingPermission 交互策略", "Interaktive TestingPermission-Richtlinie", "TestingPermission の対話ポリシー", "TestingPermission 대화형 정책", "Интерактивная политика TestingPermission")
	add(KeyPermissionDenyTestingPrompt,
		"Denied by the TestingPermission prompt", "已被 TestingPermission 提示拒绝", "Durch die TestingPermission-Abfrage abgelehnt", "TestingPermission の確認で拒否されました", "TestingPermission 확인에서 거부되었습니다", "Отклонено в запросе TestingPermission")
	add(KeyPermissionDenySnapshotAsk,
		"Denied by a spawn-time ask rule from the sub-agent snapshot", "已被 sub-agent 权限快照中的启动时询问规则拒绝", "Durch eine Ask-Regel im Start-Snapshot des Sub-Agents abgelehnt", "sub-agent の起動時スナップショットにある確認ルールで拒否されました", "sub-agent 시작 시점 스냅샷의 확인 규칙에 의해 거부되었습니다", "Отклонено правилом запроса из снимка sub-agent на момент запуска")
	add(KeyPermissionAskAlwaysPolicy,
		"Ask-always permission mode", "始终询问权限模式", "Berechtigungsmodus „Immer fragen“", "常に確認する権限モード", "항상 확인 권한 모드", "Режим разрешений «Всегда спрашивать»")
	add(KeyPermissionDenyAskAlways,
		"Denied in ask-always mode because no prompt was available or the user rejected it", "在始终询问模式下被拒绝：无法显示提示或用户已拒绝", "Im Modus „Immer fragen“ abgelehnt, weil keine Abfrage verfügbar war oder der Benutzer sie abgelehnt hat", "常に確認するモードで、確認を表示できなかったかユーザーが拒否したため拒否されました", "항상 확인 모드에서 확인을 표시할 수 없거나 사용자가 거부하여 거부되었습니다", "Отклонено в режиме «Всегда спрашивать»: запрос недоступен или пользователь отказал")
	add(KeyPermissionDenyRule,
		"Denied by a permission rule", "已被权限规则拒绝", "Durch eine Berechtigungsregel abgelehnt", "権限ルールにより拒否されました", "권한 규칙에 의해 거부되었습니다", "Отклонено правилом разрешений")
	add(KeyPermissionRuleFallback,
		"Rule-based fallback: no matching permission rule", "基于规则的回退：没有匹配的权限规则", "Regelbasierter Fallback: keine passende Berechtigungsregel", "ルールベースの fallback: 一致する権限ルールがありません", "규칙 기반 fallback: 일치하는 권한 규칙 없음", "Fallback по правилам: подходящее правило разрешений отсутствует")
	add(KeyPermissionAdvisoryPolicy,
		"Advisory safety policy", "建议性安全策略", "Hinweisbasierte Sicherheitsrichtlinie", "助言的な安全ポリシー", "권고 안전 정책", "Рекомендательная политика безопасности")
	add(KeyPermissionMandatoryPolicy,
		"Mandatory approval policy", "强制批准策略", "Verbindliche Genehmigungsrichtlinie", "必須承認ポリシー", "필수 승인 정책", "Политика обязательного подтверждения")
	add(KeyPermissionApprovalRequired,
		"Approval required: %s", "需要批准：%s", "Genehmigung erforderlich: %s", "承認が必要です: %s", "승인 필요: %s", "Требуется подтверждение: %s")
	add(KeyPermissionSnapshotSource,
		"Sub-agent spawn-time permission snapshot", "sub-agent 启动时权限快照", "Berechtigungs-Snapshot des Sub-Agents beim Start", "sub-agent 起動時の権限スナップショット", "sub-agent 시작 시점 권한 스냅샷", "Снимок разрешений sub-agent на момент запуска")
	add(KeyPermissionConfiguredRule,
		"Configured permission rule (tool=%q, decision=ask)", "已配置的权限规则（工具=%q，决定=询问）", "Konfigurierte Berechtigungsregel (Tool=%q, Entscheidung=fragen)", "設定済みの権限ルール（ツール=%q、判断=確認）", "구성된 권한 규칙(도구=%q, 결정=확인)", "Настроенное правило разрешений (инструмент=%q, решение=запросить)")
	add(KeyPermissionConfiguredPatternRule,
		"Configured permission rule (tool=%q, pattern=%q, decision=ask)", "已配置的权限规则（工具=%q，模式=%q，决定=询问）", "Konfigurierte Berechtigungsregel (Tool=%q, Muster=%q, Entscheidung=fragen)", "設定済みの権限ルール（ツール=%q、パターン=%q、判断=確認）", "구성된 권한 규칙(도구=%q, 패턴=%q, 결정=확인)", "Настроенное правило разрешений (инструмент=%q, шаблон=%q, решение=запросить)")
	add(KeyPermissionSafetyProtectedPath,
		"Write to protected path: %s", "写入受保护路径：%s", "Schreiben in geschützten Pfad: %s", "保護されたパスへの書き込み: %s", "보호된 경로에 쓰기: %s", "Запись в защищённый путь: %s")
	add(KeyPermissionSafetyUnavailable,
		"Bash safety checks are not initialized; access is denied by default", "Bash 安全检查尚未初始化；默认拒绝访问", "Bash-Sicherheitsprüfungen sind nicht initialisiert; der Zugriff wird standardmäßig abgelehnt", "Bash の安全確認が初期化されていないため、既定で拒否します", "Bash 안전 검사가 초기화되지 않아 기본적으로 거부됩니다", "Проверки безопасности Bash не инициализированы; доступ по умолчанию запрещён")
	add(KeyPermissionSafetyDangerousCommand,
		"Dangerous command: %s", "危险命令：%s", "Gefährlicher Befehl: %s", "危険なコマンド: %s", "위험한 명령: %s", "Опасная команда: %s")
	add(KeyPermissionSafetyShellProtectedPath,
		"Shell write to protected path: %s", "Shell 写入受保护路径：%s", "Shell-Schreibzugriff auf geschützten Pfad: %s", "Shell による保護されたパスへの書き込み: %s", "Shell의 보호된 경로 쓰기: %s", "Запись Shell в защищённый путь: %s")
	add(KeyPermissionSafetyPowerShell,
		"Dangerous PowerShell command: %s", "危险的 PowerShell 命令：%s", "Gefährlicher PowerShell-Befehl: %s", "危険な PowerShell コマンド: %s", "위험한 PowerShell 명령: %s", "Опасная команда PowerShell: %s")
	add(KeyPermissionPreviewSendMessage,
		"to %s: %s", "发送给 %s：%s", "an %s: %s", "%s へ: %s", "%s에게: %s", "для %s: %s")
	add(KeyPermissionPreviewSendTarget,
		"to %s", "发送给 %s", "an %s", "%s へ", "%s에게", "для %s")
	add(KeyPermissionEnvironmentRoot,
		"--allow-all cannot run as root outside a container; use --sandbox or run in Docker", "--allow-all 不能在容器外以 root 身份运行；请使用 --sandbox 或在 Docker 中运行", "--allow-all kann außerhalb eines Containers nicht als root ausgeführt werden; verwende --sandbox oder Docker", "--allow-all はコンテナ外で root として実行できません。--sandbox を使用するか Docker 内で実行してください", "--allow-all은 컨테이너 외부에서 root로 실행할 수 없습니다. --sandbox를 사용하거나 Docker에서 실행하세요", "--allow-all нельзя запускать от root вне контейнера; используйте --sandbox или Docker")
}
