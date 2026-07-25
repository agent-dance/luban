package i18n

const (
	KeyPermissionModeAllowAllFrozen    Key = "permission.mode.allow_all_frozen"
	KeyPermissionAskAlwaysPolicy       Key = "permission.policy.ask_always"
	KeyPermissionRuleFallback          Key = "permission.policy.rule_fallback"
	KeyPermissionMandatoryPolicy       Key = "permission.policy.mandatory"
	KeyPermissionApprovalRequired      Key = "permission.deny.approval_required"
	KeyPermissionSnapshotSource        Key = "permission.policy.snapshot"
	KeyPermissionConfiguredRule        Key = "permission.policy.configured_rule"
	KeyPermissionConfiguredPatternRule Key = "permission.policy.configured_pattern_rule"
	KeyPermissionSafetyProtectedPath   Key = "permission.safety.protected_path"
	KeyPermissionSafetyUnavailable     Key = "permission.safety.unavailable"
	KeyPermissionSafetyPowerShell      Key = "permission.safety.powershell"
	KeyPermissionPreviewSendMessage    Key = "permission.preview.send_message"
	KeyPermissionPreviewSendTarget     Key = "permission.preview.send_target"
	KeyPermissionEnvironmentRoot       Key = "permission.environment.root"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyPermissionModeAllowAllFrozen,
		"Cannot enable Allow All mode after the session has started", "session 启动后无法启用全部允许模式", "Der Modus „Alles erlauben“ kann nach dem Sitzungsstart nicht aktiviert werden", "セッション開始後は「すべて許可」モードを有効にできません", "세션이 시작된 후에는 모두 허용 모드를 활성화할 수 없습니다", "После запуска сеанса нельзя включить режим «Разрешить всё»")
	add(KeyPermissionAskAlwaysPolicy,
		"Ask-always permission mode", "始终询问权限模式", "Berechtigungsmodus „Immer fragen“", "常に確認する権限モード", "항상 확인 권한 모드", "Режим разрешений «Всегда спрашивать»")
	add(KeyPermissionRuleFallback,
		"Rule-based fallback: no matching permission rule", "基于规则的回退：没有匹配的权限规则", "Regelbasierter Fallback: keine passende Berechtigungsregel", "ルールベースの fallback: 一致する権限ルールがありません", "규칙 기반 fallback: 일치하는 권한 규칙 없음", "Fallback по правилам: подходящее правило разрешений отсутствует")
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
	add(KeyPermissionSafetyPowerShell,
		"Dangerous PowerShell command: %s", "危险的 PowerShell 命令：%s", "Gefährlicher PowerShell-Befehl: %s", "危険な PowerShell コマンド: %s", "위험한 PowerShell 명령: %s", "Опасная команда PowerShell: %s")
	add(KeyPermissionPreviewSendMessage,
		"to %s: %s", "发送给 %s：%s", "an %s: %s", "%s へ: %s", "%s에게: %s", "для %s: %s")
	add(KeyPermissionPreviewSendTarget,
		"to %s", "发送给 %s", "an %s", "%s へ", "%s에게", "для %s")
	add(KeyPermissionEnvironmentRoot,
		"--allow-all cannot run as root outside a container; use --sandbox or run in Docker", "--allow-all 不能在容器外以 root 身份运行；请使用 --sandbox 或在 Docker 中运行", "--allow-all kann außerhalb eines Containers nicht als root ausgeführt werden; verwende --sandbox oder Docker", "--allow-all はコンテナ外で root として実行できません。--sandbox を使用するか Docker 内で実行してください", "--allow-all은 컨테이너 외부에서 root로 실행할 수 없습니다. --sandbox를 사용하거나 Docker에서 실행하세요", "--allow-all нельзя запускать от root вне контейнера; используйте --sandbox или Docker")
}
