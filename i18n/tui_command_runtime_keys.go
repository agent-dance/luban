package i18n

// Semantic copy shared by command, fullscreen TUI, terminal, and screen-reader
// presentation boundaries. Stable IDs, slash commands, protocol names, paths,
// and raw external values remain parameters and are never translated here.
const (
	KeyTUIToolErrorHeader           Key = "tui.tool_error.header"
	KeyTUIToolFallback              Key = "tui.tool_error.fallback_tool"
	KeyTUIToolPermissionDenied      Key = "tui.tool_error.permission_denied"
	KeyTUIToolPermissionCheckFailed Key = "tui.tool_error.permission_check_failed"
	KeyTUIToolCancelledByUser       Key = "tui.tool_error.cancelled_by_user"
	KeyTUIToolContextCancelled      Key = "tui.tool_error.context_cancelled"
	KeyTUIToolExecutionCancelled    Key = "tui.tool_error.execution_cancelled"
	KeyTUIToolFailed                Key = "tui.tool_error.failed"

	KeyRuntimeActivityKindTool       Key = "runtime.activity.kind.tool"
	KeyRuntimeActivityKindCommand    Key = "runtime.activity.kind.command"
	KeyRuntimeActivityKindAgent      Key = "runtime.activity.kind.agent"
	KeyRuntimeActivityKindBackground Key = "runtime.activity.kind.background"
	KeyRuntimeActivityKindDecision   Key = "runtime.activity.kind.decision"
	KeyRuntimeActivityKindHook       Key = "runtime.activity.kind.hook"
	KeyRuntimeActivitySpawning       Key = "runtime.activity.state.spawning"
	KeyRuntimeActivityQueued         Key = "runtime.activity.state.queued"
	KeyRuntimeActivityWaiting        Key = "runtime.activity.state.waiting"
	KeyRuntimeActivityBlocked        Key = "runtime.activity.state.blocked"
	KeyRuntimeActivityNeedsInput     Key = "runtime.activity.state.needs_input"
	KeyRuntimeActivityReadyReview    Key = "runtime.activity.state.ready_review"
	KeyRuntimeActivityActionCancel   Key = "runtime.activity.action.cancel"
	KeyRuntimeActivityActionJump     Key = "runtime.activity.action.jump"
	KeyRuntimeActivityActionDetails  Key = "runtime.activity.action.details"
	KeyRuntimeActivityActionAck      Key = "runtime.activity.action.acknowledge"
	KeyRuntimeActivityCurrent        Key = "runtime.activity.progress.current"
	KeyRuntimePlanPrompt             Key = "runtime.mode.plan_prompt"

	KeyRuntimeProviderConnected    Key = "runtime.provider.connected"
	KeyRuntimeProviderDisconnected Key = "runtime.provider.disconnected"
	KeyRuntimeProviderError        Key = "runtime.provider.error"
	KeyRuntimeProviderUnknown      Key = "runtime.provider.unknown"

	KeyRuntimeRiskLow                        Key = "runtime.risk.low"
	KeyRuntimeDecisionScopeRule              Key = "runtime.decision.scope_rule"
	KeyRuntimeDecisionEvidenceName           Key = "runtime.decision.evidence_name"
	KeyRuntimeDecisionSuppliedInput          Key = "runtime.decision.supplied_input"
	KeyRuntimeDecisionReceiptLine            Key = "runtime.decision.receipt_line"
	KeyRuntimeDecisionApproved               Key = "runtime.decision.outcome.approved"
	KeyRuntimeDecisionRejected               Key = "runtime.decision.outcome.rejected"
	KeyRuntimeDecisionEscaped                Key = "runtime.decision.outcome.escaped"
	KeyRuntimeDecisionShutdown               Key = "runtime.decision.outcome.shutdown"
	KeyRuntimeDecisionChoiceNone             Key = "runtime.decision.choice.none"
	KeyRuntimeDecisionChoiceSubmit           Key = "runtime.decision.choice.submit"
	KeyRuntimePromptKindPermission           Key = "runtime.decision.kind.permission"
	KeyRuntimePromptKindPlan                 Key = "runtime.decision.kind.plan"
	KeyRuntimePromptKindAskUser              Key = "runtime.decision.kind.ask_user"
	KeyRuntimePermissionReviewNormalizedPath Key = "runtime.permission.review.normalized_path"
	KeyRuntimePermissionReviewAllowedDir     Key = "runtime.permission.review.allowed_directory"
	KeyRuntimePermissionReviewAccess         Key = "runtime.permission.review.access"
	KeyRuntimePermissionAccessReadOnly       Key = "runtime.permission.access.read_only"
	KeyRuntimePermissionAccessWrite          Key = "runtime.permission.access.write"
	KeyRuntimePermissionAccessExecute        Key = "runtime.permission.access.execute"
	KeyRuntimePermissionReviewMatchedRule    Key = "runtime.permission.review.matched_rule"
	KeyRuntimePermissionReviewRequiredScope  Key = "runtime.permission.review.required_scope"

	KeyRuntimePresentationLevelHidden     Key = "runtime.presentation.level.hidden"
	KeyRuntimePresentationLevelFolded     Key = "runtime.presentation.level.folded"
	KeyRuntimePresentationLevelStructured Key = "runtime.presentation.level.structured"
	KeyRuntimePresentationLevelEvidence   Key = "runtime.presentation.level.evidence"
	KeyRuntimePresentationFlagEdited      Key = "runtime.presentation.flag.edited"
	KeyRuntimePresentationFlagAgent       Key = "runtime.presentation.flag.agent"
	KeyRuntimePresentationFlagRecurring   Key = "runtime.presentation.flag.recurring"
	KeyRuntimePresentationFlagDurable     Key = "runtime.presentation.flag.durable"

	KeyRuntimeCodePlainText      Key = "runtime.code.plain_text"
	KeyRuntimeCodeLineCount      Key = "runtime.code.line_count"
	KeyRuntimeAssistantPreview   Key = "runtime.session.assistant_preview"
	KeyRuntimePersistedImage     Key = "runtime.session.persisted_image"
	KeyRuntimePersistedDocument  Key = "runtime.session.persisted_document"
	KeyRuntimePersistedTool      Key = "runtime.session.persisted_tool"
	KeyRuntimeContentReplacement Key = "runtime.session.content_replacement"

	KeyRuntimeSkillVisibilityAuto       Key = "runtime.skill.visibility.auto"
	KeyRuntimeSkillVisibilityNameOnly   Key = "runtime.skill.visibility.name_only"
	KeyRuntimeSkillVisibilityManualOnly Key = "runtime.skill.visibility.manual_only"
	KeyRuntimeSkillVisibilityOff        Key = "runtime.skill.visibility.off"
	KeyRuntimeSkillScopeDefault         Key = "runtime.skill.scope.default"
	KeyRuntimeSkillScopeFrontmatter     Key = "runtime.skill.scope.frontmatter"
	KeyRuntimeSkillScopeUser            Key = "runtime.skill.scope.user"
	KeyRuntimeSkillScopeProject         Key = "runtime.skill.scope.project"
	KeyRuntimeSkillScopeSession         Key = "runtime.skill.scope.session"
	KeyRuntimeSkillScopeManaged         Key = "runtime.skill.scope.managed"
	KeyRuntimeSkillSourceProject        Key = "runtime.skill.source.project"
	KeyRuntimeSkillSourceUser           Key = "runtime.skill.source.user"
	KeyRuntimeSkillSourceManaged        Key = "runtime.skill.source.managed"
	KeyRuntimeSkillSourcePlugin         Key = "runtime.skill.source.plugin"
	KeyRuntimeSkillSourceBundled        Key = "runtime.skill.source.bundled"
	KeyRuntimeSkillContextInline        Key = "runtime.skill.context.inline"
	KeyRuntimeSkillContextFork          Key = "runtime.skill.context.fork"

	KeyRuntimeProjectInstructionsTemplate Key = "runtime.command.project_instructions_template"
	KeyRuntimeDoctorUnknownVersion        Key = "runtime.command.doctor.unknown_version"
	KeyRuntimeSettingsParseError          Key = "runtime.command.settings_parse_error"
	KeyRuntimeCommandActivityActionFailed Key = "runtime.command.activity_action_failed"
	KeyRuntimeCommandDetailFailed         Key = "runtime.command.detail_failed"
	KeyRuntimeCommandSearchFailed         Key = "runtime.command.search_failed"
	KeyRuntimeCommandExportFailed         Key = "runtime.command.export_failed"
	KeyRuntimeCommandEditorFailed         Key = "runtime.command.editor_failed"
	KeyRuntimeCommandMouseFailed          Key = "runtime.command.mouse_failed"
	KeyRuntimeCommandSessionDeleteFailed  Key = "runtime.command.session_delete_failed"

	KeyRuntimeEditorEnvMissing        Key = "runtime.editor.env_missing"
	KeyRuntimeEditorCommandEmpty      Key = "runtime.editor.command_empty"
	KeyRuntimeEditorIncompleteEscape  Key = "runtime.editor.incomplete_escape"
	KeyRuntimeEditorUnclosedQuote     Key = "runtime.editor.unclosed_quote"
	KeyRuntimeClipboardCommandMissing Key = "runtime.clipboard.command_missing"
	KeyRuntimeClipboardUnsupportedOS  Key = "runtime.clipboard.unsupported_os"

	KeyRuntimeWeekdaySunday    Key = "runtime.time.weekday.sunday"
	KeyRuntimeWeekdayMonday    Key = "runtime.time.weekday.monday"
	KeyRuntimeWeekdayTuesday   Key = "runtime.time.weekday.tuesday"
	KeyRuntimeWeekdayWednesday Key = "runtime.time.weekday.wednesday"
	KeyRuntimeWeekdayThursday  Key = "runtime.time.weekday.thursday"
	KeyRuntimeWeekdayFriday    Key = "runtime.time.weekday.friday"
	KeyRuntimeWeekdaySaturday  Key = "runtime.time.weekday.saturday"
	KeyRuntimeRecentTimestamp  Key = "runtime.time.recent"

	KeyRuntimeCacheBreakDebug Key = "runtime.cache_break_debug"
	KeyRuntimeJSONMarshalLog  Key = "runtime.json.marshal_error"
	KeyRuntimeJSONWriteLog    Key = "runtime.json.write_error"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyTUIToolErrorHeader, "Error:", "错误：", "Fehler:", "エラー:", "오류:", "Ошибка:")
	add(KeyTUIToolFallback, "Tool", "工具", "Tool", "ツール", "도구", "Инструмент")
	add(KeyTUIToolPermissionDenied, "%s permission denied", "%s 权限被拒绝", "Berechtigung für %s verweigert", "%s の権限が拒否されました", "%s 권한이 거부됨", "Доступ для %s запрещён")
	add(KeyTUIToolPermissionCheckFailed, "%s permission check failed", "%s 权限检查失败", "Berechtigungsprüfung für %s fehlgeschlagen", "%s の権限確認に失敗しました", "%s 권한 확인 실패", "Не удалось проверить разрешение для %s")
	add(KeyTUIToolCancelledByUser, "%s cancelled by user", "用户取消了 %s", "%s vom Benutzer abgebrochen", "%s はユーザーによりキャンセルされました", "사용자가 %s을(를) 취소함", "%s отменён пользователем")
	add(KeyTUIToolContextCancelled, "%s context cancelled", "%s 上下文已取消", "Kontext für %s abgebrochen", "%s のコンテキストがキャンセルされました", "%s 컨텍스트 취소됨", "Контекст для %s отменён")
	add(KeyTUIToolExecutionCancelled, "%s execution cancelled", "%s 执行已取消", "Ausführung von %s abgebrochen", "%s の実行がキャンセルされました", "%s 실행 취소됨", "Выполнение %s отменено")
	add(KeyTUIToolFailed, "%s failed", "%s 失败", "%s fehlgeschlagen", "%s に失敗しました", "%s 실패", "Сбой %s")

	add(KeyRuntimeActivityKindTool, "tool", "工具", "Tool", "ツール", "도구", "инструмент")
	add(KeyRuntimeActivityKindCommand, "command", "命令", "Befehl", "コマンド", "명령", "команда")
	add(KeyRuntimeActivityKindAgent, "agent", "Agent", "Agent", "Agent", "Agent", "Agent")
	add(KeyRuntimeActivityKindBackground, "background task", "后台任务", "Hintergrundaufgabe", "バックグラウンドタスク", "백그라운드 작업", "фоновая задача")
	add(KeyRuntimeActivityKindDecision, "decision", "决策", "Entscheidung", "決定", "결정", "решение")
	add(KeyRuntimeActivityKindHook, "hook", "Hook", "Hook", "Hook", "Hook", "Hook")
	add(KeyRuntimeActivitySpawning, "starting", "正在启动", "wird gestartet", "開始中", "시작 중", "запускается")
	add(KeyRuntimeActivityQueued, "queued", "已排队", "in Warteschlange", "待機中", "대기 중", "в очереди")
	add(KeyRuntimeActivityWaiting, "waiting", "等待中", "wartet", "待機中", "대기 중", "ожидает")
	add(KeyRuntimeActivityBlocked, "blocked", "已阻塞", "blockiert", "ブロック中", "차단됨", "заблокировано")
	add(KeyRuntimeActivityNeedsInput, "needs input", "需要输入", "Eingabe erforderlich", "入力が必要", "입력 필요", "требуется ввод")
	add(KeyRuntimeActivityReadyReview, "ready for review", "等待审核", "bereit zur Prüfung", "レビュー待ち", "검토 준비됨", "готово к проверке")
	add(KeyRuntimeActivityActionCancel, "cancel", "取消", "abbrechen", "キャンセル", "취소", "отменить")
	add(KeyRuntimeActivityActionJump, "jump", "跳转", "öffnen", "移動", "이동", "перейти")
	add(KeyRuntimeActivityActionDetails, "details", "详情", "Details", "詳細", "세부 정보", "подробности")
	add(KeyRuntimeActivityActionAck, "acknowledge", "确认", "bestätigen", "確認", "확인", "подтвердить")
	add(KeyRuntimeActivityCurrent, "current=%d", "当前=%d", "aktuell=%d", "現在=%d", "현재=%d", "текущее=%d")
	add(KeyRuntimePlanPrompt, "plan> ", "计划> ", "Plan> ", "計画> ", "계획> ", "план> ")

	add(KeyRuntimeProviderConnected, "connected", "已连接", "verbunden", "接続済み", "연결됨", "подключён")
	add(KeyRuntimeProviderDisconnected, "disconnected", "未连接", "nicht verbunden", "未接続", "연결 끊김", "не подключён")
	add(KeyRuntimeProviderError, "connection error", "连接错误", "Verbindungsfehler", "接続エラー", "연결 오류", "ошибка подключения")
	add(KeyRuntimeProviderUnknown, "unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно")

	add(KeyRuntimeRiskLow, "low", "低", "niedrig", "低", "낮음", "низкий")
	add(KeyRuntimeDecisionScopeRule, "%s  Rule: %s", "%s  规则：%s", "%s  Regel: %s", "%s  ルール: %s", "%s  규칙: %s", "%s  Правило: %s")
	add(KeyRuntimeDecisionEvidenceName, "Decision %s", "决策 %s", "Entscheidung %s", "決定 %s", "결정 %s", "Решение %s")
	add(KeyRuntimeDecisionSuppliedInput, "supplied input", "提供的输入", "bereitgestellte Eingabe", "指定された入力", "제공된 입력", "переданные данные")
	add(KeyRuntimeDecisionReceiptLine, "%s: %s", "%s：%s", "%s: %s", "%s: %s", "%s: %s", "%s: %s")
	add(KeyRuntimeDecisionApproved, "approved", "已批准", "genehmigt", "承認済み", "승인됨", "одобрено")
	add(KeyRuntimeDecisionRejected, "rejected", "已拒绝", "abgelehnt", "拒否", "거부됨", "отклонено")
	add(KeyRuntimeDecisionEscaped, "dismissed", "已关闭", "geschlossen", "閉じました", "닫힘", "закрыто")
	add(KeyRuntimeDecisionShutdown, "stopped during shutdown", "因关闭而停止", "beim Beenden abgebrochen", "終了処理中に停止", "종료 중 중지됨", "остановлено при завершении")
	add(KeyRuntimeDecisionChoiceNone, "none", "无", "keine", "なし", "없음", "нет")
	add(KeyRuntimeDecisionChoiceSubmit, "submitted", "已提交", "gesendet", "送信済み", "제출됨", "отправлено")
	add(KeyRuntimePromptKindPermission, "permission", "权限", "Berechtigung", "権限", "권한", "разрешение")
	add(KeyRuntimePromptKindPlan, "plan", "计划", "Plan", "計画", "계획", "план")
	add(KeyRuntimePromptKindAskUser, "question", "提问", "Frage", "質問", "질문", "вопрос")
	add(KeyRuntimePermissionReviewNormalizedPath, "Normalized path: %s", "规范化路径：%s", "Normalisierter Pfad: %s", "正規化されたパス: %s", "정규화된 경로: %s", "Нормализованный путь: %s")
	add(KeyRuntimePermissionReviewAllowedDir, "Allowed directory: %s", "允许目录：%s", "Zulässiges Verzeichnis: %s", "許可されたディレクトリ: %s", "허용된 디렉터리: %s", "Разрешённый каталог: %s")
	add(KeyRuntimePermissionReviewAccess, "Access: %s", "访问类型：%s", "Zugriff: %s", "アクセス: %s", "접근 유형: %s", "Доступ: %s")
	add(KeyRuntimePermissionAccessReadOnly, "read-only", "只读", "schreibgeschützt", "読み取り専用", "읽기 전용", "только чтение")
	add(KeyRuntimePermissionAccessWrite, "write", "写入", "Schreiben", "書き込み", "쓰기", "запись")
	add(KeyRuntimePermissionAccessExecute, "execute", "执行", "Ausführen", "実行", "실행", "выполнение")
	add(KeyRuntimePermissionReviewMatchedRule, "Matched rule: %s", "命中的规则：%s", "Angewendete Regel: %s", "一致したルール: %s", "일치한 규칙: %s", "Применённое правило: %s")
	add(KeyRuntimePermissionReviewRequiredScope, "Required approval scope: this invocation", "强制审批范围：仅本次调用", "Umfang der erforderlichen Genehmigung: dieser Aufruf", "必須承認の範囲: この呼び出しのみ", "필수 승인 범위: 이번 호출", "Область обязательного подтверждения: только этот вызов")

	add(KeyRuntimePresentationLevelHidden, "hidden member", "隐藏成员", "ausgeblendetes Mitglied", "非表示メンバー", "숨겨진 항목", "скрытый элемент")
	add(KeyRuntimePresentationLevelFolded, "collapsed", "已折叠", "eingeklappt", "折りたたみ", "접힘", "свёрнуто")
	add(KeyRuntimePresentationLevelStructured, "summary and details", "摘要和详情", "Zusammenfassung und Details", "概要と詳細", "요약 및 세부 정보", "сводка и подробности")
	add(KeyRuntimePresentationLevelEvidence, "retained evidence", "已保留证据", "gespeicherte Belege", "保持された証拠", "보존된 근거", "сохранённые данные")
	add(KeyRuntimePresentationFlagEdited, "edited=%t", "已编辑=%t", "bearbeitet=%t", "編集済み=%t", "편집됨=%t", "изменено=%t")
	add(KeyRuntimePresentationFlagAgent, "agent=%t", "Agent=%t", "Agent=%t", "Agent=%t", "Agent=%t", "Agent=%t")
	add(KeyRuntimePresentationFlagRecurring, "recurring=%t", "重复执行=%t", "wiederkehrend=%t", "繰り返し=%t", "반복=%t", "повторяется=%t")
	add(KeyRuntimePresentationFlagDurable, "durable=%t", "持久化=%t", "dauerhaft=%t", "永続=%t", "영구=%t", "постоянно=%t")

	add(KeyRuntimeCodePlainText, "Plain text", "纯文本", "Klartext", "プレーンテキスト", "일반 텍스트", "Обычный текст")
	add(KeyRuntimeCodeLineCount, "%d lines", "%d 行", "%d Zeilen", "%d 行", "%d줄", "%d строк")
	add(KeyRuntimeAssistantPreview, "Assistant: %s", "助手：%s", "Assistent: %s", "アシスタント: %s", "어시스턴트: %s", "Ассистент: %s")
	add(KeyRuntimePersistedImage, "[image]", "[图片]", "[Bild]", "[画像]", "[이미지]", "[изображение]")
	add(KeyRuntimePersistedDocument, "[document]", "[文档]", "[Dokument]", "[ドキュメント]", "[문서]", "[документ]")
	add(KeyRuntimePersistedTool, "[tool: %s]", "[工具：%s]", "[Tool: %s]", "[ツール: %s]", "[도구: %s]", "[инструмент: %s]")
	add(KeyRuntimeContentReplacement, "[content replacement]", "[内容替换]", "[Inhaltsersetzung]", "[コンテンツ置換]", "[콘텐츠 교체]", "[замена содержимого]")

	add(KeyRuntimeSkillVisibilityAuto, "automatic", "自动", "automatisch", "自動", "자동", "автоматически")
	add(KeyRuntimeSkillVisibilityNameOnly, "name-only", "仅名称", "nur Name", "名前のみ", "이름만", "только имя")
	add(KeyRuntimeSkillVisibilityManualOnly, "manual-only", "仅手动", "nur manuell", "手動のみ", "수동 전용", "только вручную")
	add(KeyRuntimeSkillVisibilityOff, "off", "关闭", "aus", "オフ", "꺼짐", "выключено")
	add(KeyRuntimeSkillScopeDefault, "default", "默认", "Standard", "デフォルト", "기본값", "по умолчанию")
	add(KeyRuntimeSkillScopeFrontmatter, "skill frontmatter", "技能 frontmatter", "Skill-Frontmatter", "スキルの frontmatter", "스킬 frontmatter", "frontmatter навыка")
	add(KeyRuntimeSkillScopeUser, "user", "用户", "Benutzer", "ユーザー", "사용자", "пользователь")
	add(KeyRuntimeSkillScopeProject, "project", "项目", "Projekt", "プロジェクト", "프로젝트", "проект")
	add(KeyRuntimeSkillScopeSession, "session", "会话", "Sitzung", "セッション", "세션", "сеанс")
	add(KeyRuntimeSkillScopeManaged, "managed policy", "托管策略", "verwaltete Richtlinie", "管理ポリシー", "관리 정책", "управляемая политика")
	add(KeyRuntimeSkillSourceProject, "project", "项目", "Projekt", "プロジェクト", "프로젝트", "проект")
	add(KeyRuntimeSkillSourceUser, "user", "用户", "Benutzer", "ユーザー", "사용자", "пользователь")
	add(KeyRuntimeSkillSourceManaged, "managed", "托管", "verwaltet", "管理対象", "관리됨", "управляемый")
	add(KeyRuntimeSkillSourcePlugin, "plugin", "插件", "Plugin", "プラグイン", "플러그인", "плагин")
	add(KeyRuntimeSkillSourceBundled, "bundled", "内置", "mitgeliefert", "同梱", "기본 제공", "встроенный")
	add(KeyRuntimeSkillContextInline, "current conversation", "当前对话", "aktuelle Unterhaltung", "現在の会話", "현재 대화", "текущий диалог")
	add(KeyRuntimeSkillContextFork, "isolated Agent", "隔离的 Agent", "isolierter Agent", "分離された Agent", "격리된 Agent", "изолированный Agent")

	add(KeyRuntimeProjectInstructionsTemplate, "# Project Instructions\n\nAdd project-specific instructions for LUBAN Code here.\nThese instructions are loaded at the start of every conversation.\n\n## Examples\n- \"Always use TypeScript strict mode\"\n- \"Run tests with: go test ./...\"\n- \"Follow the existing code style in src/\"\n", "# 项目说明\n\n请在此添加 LUBAN Code 的项目专用说明。\n每次开始对话时都会加载这些说明。\n\n## 示例\n- \"始终启用 TypeScript strict mode\"\n- \"运行测试：go test ./...\"\n- \"遵循 src/ 中现有的代码风格\"\n", "# Projektanweisungen\n\nFüge hier projektspezifische Anweisungen für LUBAN Code hinzu.\nDiese Anweisungen werden zu Beginn jeder Unterhaltung geladen.\n\n## Beispiele\n- \"TypeScript immer im strict mode verwenden\"\n- \"Tests ausführen mit: go test ./...\"\n- \"Dem bestehenden Codestil in src/ folgen\"\n", "# プロジェクト指示\n\nLUBAN Code 用のプロジェクト固有の指示をここに追加してください。\nこの指示は会話の開始時に毎回読み込まれます。\n\n## 例\n- \"TypeScript の strict mode を常に使用する\"\n- \"テストを実行: go test ./...\"\n- \"src/ の既存のコードスタイルに従う\"\n", "# 프로젝트 지침\n\nLUBAN Code용 프로젝트별 지침을 여기에 추가하세요.\n이 지침은 대화를 시작할 때마다 불러옵니다.\n\n## 예시\n- \"항상 TypeScript strict mode 사용\"\n- \"테스트 실행: go test ./...\"\n- \"src/의 기존 코드 스타일 준수\"\n", "# Инструкции проекта\n\nДобавьте здесь инструкции проекта для LUBAN Code.\nОни загружаются в начале каждого диалога.\n\n## Примеры\n- \"Всегда использовать strict mode в TypeScript\"\n- \"Запускать тесты командой: go test ./...\"\n- \"Следовать существующему стилю кода в src/\"\n")
	add(KeyRuntimeDoctorUnknownVersion, "unknown version", "版本未知", "unbekannte Version", "バージョン不明", "버전 알 수 없음", "версия неизвестна")
	add(KeyRuntimeSettingsParseError, "Could not parse settings.json: %v", "无法解析 settings.json：%v", "settings.json konnte nicht gelesen werden: %v", "settings.json を解析できませんでした: %v", "settings.json을 파싱할 수 없습니다: %v", "Не удалось разобрать settings.json: %v")
	add(KeyRuntimeCommandActivityActionFailed, "Could not %s activity %s", "无法执行活动操作“%s”（活动 %s）", "Die Aktivitätsaktion „%s“ für %s konnte nicht ausgeführt werden", "アクティビティ操作「%s」を %s に対して実行できませんでした", "활동 작업 ‘%s’을(를) %s에 실행할 수 없습니다", "Не удалось выполнить действие «%s» для активности %s")
	add(KeyRuntimeCommandDetailFailed, "Could not change details for %s", "无法更改 %s 的详细信息", "Details für %s konnten nicht geändert werden", "%s の詳細表示を変更できませんでした", "%s의 세부 정보를 변경할 수 없습니다", "Не удалось изменить детализацию для %s")
	add(KeyRuntimeCommandSearchFailed, "Could not search the transcript", "无法搜索对话记录", "Das Gespräch konnte nicht durchsucht werden", "会話履歴を検索できませんでした", "대화 기록을 검색할 수 없습니다", "Не удалось выполнить поиск по истории диалога")
	add(KeyRuntimeCommandExportFailed, "Could not export the transcript", "无法导出对话记录", "Das Gespräch konnte nicht exportiert werden", "会話履歴をエクスポートできませんでした", "대화 기록을 내보낼 수 없습니다", "Не удалось экспортировать историю диалога")
	add(KeyRuntimeCommandEditorFailed, "Could not open the editor", "无法打开编辑器", "Der Editor konnte nicht geöffnet werden", "エディターを開けませんでした", "편집기를 열 수 없습니다", "Не удалось открыть редактор")
	add(KeyRuntimeCommandMouseFailed, "Could not update mouse capture", "无法更新鼠标捕获设置", "Die Mauserfassung konnte nicht geändert werden", "マウスキャプチャを変更できませんでした", "마우스 캡처를 변경할 수 없습니다", "Не удалось изменить захват мыши")
	add(KeyRuntimeCommandSessionDeleteFailed, "Could not delete session %s", "无法删除会话 %s", "Sitzung %s konnte nicht gelöscht werden", "セッション %s を削除できませんでした", "세션 %s을(를) 삭제할 수 없습니다", "Не удалось удалить сеанс %s")

	add(KeyRuntimeEditorEnvMissing, "$VISUAL and $EDITOR are not set", "未设置 $VISUAL 和 $EDITOR", "$VISUAL und $EDITOR sind nicht gesetzt", "$VISUAL と $EDITOR が設定されていません", "$VISUAL 및 $EDITOR가 설정되지 않음", "Переменные $VISUAL и $EDITOR не заданы")
	add(KeyRuntimeEditorCommandEmpty, "Editor command is empty", "编辑器命令为空", "Der Editorbefehl ist leer", "エディターコマンドが空です", "편집기 명령이 비어 있음", "Команда редактора пуста")
	add(KeyRuntimeEditorIncompleteEscape, "Editor command ends with an incomplete escape", "编辑器命令以不完整的转义符结尾", "Der Editorbefehl endet mit einer unvollständigen Escape-Sequenz", "エディターコマンドの末尾に不完全なエスケープがあります", "편집기 명령이 불완전한 이스케이프로 끝남", "Команда редактора заканчивается незавершённой escape-последовательностью")
	add(KeyRuntimeEditorUnclosedQuote, "Editor command contains an unclosed quote", "编辑器命令包含未闭合的引号", "Der Editorbefehl enthält ein nicht geschlossenes Anführungszeichen", "エディターコマンドに閉じられていない引用符があります", "편집기 명령에 닫히지 않은 따옴표가 있음", "В команде редактора есть незакрытая кавычка")
	add(KeyRuntimeClipboardCommandMissing, "No clipboard command found; install xclip, xsel, or wl-copy", "未找到剪贴板命令；请安装 xclip、xsel 或 wl-copy", "Kein Zwischenablagebefehl gefunden; installiere xclip, xsel oder wl-copy", "クリップボード用コマンドが見つかりません。xclip、xsel、または wl-copy をインストールしてください", "클립보드 명령을 찾을 수 없습니다. xclip, xsel 또는 wl-copy를 설치하세요", "Команда буфера обмена не найдена; установите xclip, xsel или wl-copy")
	add(KeyRuntimeClipboardUnsupportedOS, "Clipboard is not supported on %s", "%s 不支持剪贴板", "Die Zwischenablage wird unter %s nicht unterstützt", "%s ではクリップボードを利用できません", "%s에서는 클립보드를 지원하지 않습니다", "Буфер обмена не поддерживается в %s")

	add(KeyRuntimeWeekdaySunday, "Sunday", "星期日", "Sonntag", "日曜日", "일요일", "воскресенье")
	add(KeyRuntimeWeekdayMonday, "Monday", "星期一", "Montag", "月曜日", "월요일", "понедельник")
	add(KeyRuntimeWeekdayTuesday, "Tuesday", "星期二", "Dienstag", "火曜日", "화요일", "вторник")
	add(KeyRuntimeWeekdayWednesday, "Wednesday", "星期三", "Mittwoch", "水曜日", "수요일", "среда")
	add(KeyRuntimeWeekdayThursday, "Thursday", "星期四", "Donnerstag", "木曜日", "목요일", "четверг")
	add(KeyRuntimeWeekdayFriday, "Friday", "星期五", "Freitag", "金曜日", "금요일", "пятница")
	add(KeyRuntimeWeekdaySaturday, "Saturday", "星期六", "Samstag", "土曜日", "토요일", "суббота")
	add(KeyRuntimeRecentTimestamp, "%s, %s", "%s %s", "%s, %s", "%s %s", "%s %s", "%s, %s")

	add(KeyRuntimeCacheBreakDebug, "[cache debug: possible cache break: call #%d, read %dK -> %dK (-%dK, -%.0f%%), created %dK, previous created %dK, gap %s]", "[cache debug：可能发生缓存中断：调用 #%d，读取 %dK -> %dK（-%dK，-%.0f%%），创建 %dK，上次创建 %dK，间隔 %s]", "[Cache-Debug: möglicher Cache-Abbruch: Aufruf #%d, gelesen %dK -> %dK (-%dK, -%.0f%%), erstellt %dK, zuvor erstellt %dK, Abstand %s]", "[cache debug: キャッシュ切断の可能性: 呼び出し #%d、読み取り %dK -> %dK（-%dK、-%.0f%%）、作成 %dK、前回作成 %dK、間隔 %s]", "[cache debug: 캐시 중단 가능성: 호출 #%d, 읽기 %dK -> %dK (-%dK, -%.0f%%), 생성 %dK, 이전 생성 %dK, 간격 %s]", "[Отладка кэша: возможный сброс: вызов #%d, чтение %dK -> %dK (-%dK, -%.0f%%), создано %dK, ранее создано %dK, интервал %s]")
	add(KeyRuntimeJSONMarshalLog, "json_renderer: could not encode output: %v", "json_renderer：无法编码输出：%v", "json_renderer: Ausgabe konnte nicht kodiert werden: %v", "json_renderer: 出力をエンコードできませんでした: %v", "json_renderer: 출력을 인코딩할 수 없음: %v", "json_renderer: не удалось закодировать вывод: %v")
	add(KeyRuntimeJSONWriteLog, "json_renderer: could not write output: %v", "json_renderer：无法写入输出：%v", "json_renderer: Ausgabe konnte nicht geschrieben werden: %v", "json_renderer: 出力を書き込めませんでした: %v", "json_renderer: 출력을 쓸 수 없음: %v", "json_renderer: не удалось записать вывод: %v")
}

func RuntimeActivityKindLabel(lang Language, code string) string {
	keys := map[string]Key{
		"tool": KeyRuntimeActivityKindTool, "command": KeyRuntimeActivityKindCommand,
		"agent": KeyRuntimeActivityKindAgent, "background": KeyRuntimeActivityKindBackground,
		"decision": KeyRuntimeActivityKindDecision, "hook": KeyRuntimeActivityKindHook,
	}
	if code == "mcp" {
		return "MCP"
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeActivityStateLabel(lang Language, code string) string {
	keys := map[string]Key{
		"started": KeyRuntimeActivitySpawning, "spawning": KeyRuntimeActivitySpawning, "queued": KeyRuntimeActivityQueued,
		"waiting": KeyRuntimeActivityWaiting, "blocked": KeyRuntimeActivityBlocked, "prevented": KeyRuntimeActivityBlocked,
		"needs_input": KeyRuntimeActivityNeedsInput, "ready_for_review": KeyRuntimeActivityReadyReview,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	switch code {
	case "running", "succeeded", "completed", "failed", "partial", "denied", "cancelled", "timed_out", "unknown":
		return TUIOutcomeLabel(lang, code)
	default:
		return code
	}
}

func RuntimeActivityActionLabel(lang Language, code string) string {
	keys := map[string]Key{
		"cancel": KeyRuntimeActivityActionCancel, "jump": KeyRuntimeActivityActionJump,
		"details": KeyRuntimeActivityActionDetails, "acknowledge": KeyRuntimeActivityActionAck,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeProviderStatusLabel(lang Language, code string) string {
	keys := map[string]Key{
		"connected": KeyRuntimeProviderConnected, "disconnected": KeyRuntimeProviderDisconnected,
		"error": KeyRuntimeProviderError, "unknown": KeyRuntimeProviderUnknown,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeDecisionOutcomeLabel(lang Language, code string) string {
	keys := map[string]Key{
		"approved": KeyRuntimeDecisionApproved, "rejected": KeyRuntimeDecisionRejected,
		"escaped": KeyRuntimeDecisionEscaped, "shutdown": KeyRuntimeDecisionShutdown,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return TUIOutcomeLabel(lang, code)
}

func RuntimeDecisionChoiceLabel(lang Language, code string) string {
	keys := map[string]Key{
		"allow_once": KeyPermissionAllowOnce, "always_allow": KeyPermissionAlwaysAllow,
		"execute": KeyPermissionExecute, "stay_in_plan": KeyPermissionStayInPlan,
		"reject": KeyPermissionReject, "none": KeyRuntimeDecisionChoiceNone, "submit": KeyRuntimeDecisionChoiceSubmit,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimePromptKindLabel(lang Language, code string) string {
	keys := map[string]Key{"permission": KeyRuntimePromptKindPermission, "plan": KeyRuntimePromptKindPlan, "ask_user": KeyRuntimePromptKindAskUser}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeModeLabel(lang Language, code string) string {
	keys := map[string]Key{
		"auto": KeyModeAuto, "auto_edit": KeyModeAuto,
		"ask": KeyModeAsk, "ask_edit": KeyModeAsk,
		"plan": KeyModePlan, "plan_edit": KeyModePlan,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimePresentationLevelLabel(lang Language, code string) string {
	keys := map[string]Key{
		"hidden_member": KeyRuntimePresentationLevelHidden, "folded": KeyRuntimePresentationLevelFolded,
		"structured": KeyRuntimePresentationLevelStructured, "evidence": KeyRuntimePresentationLevelEvidence,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeSkillVisibilityLabel(lang Language, code string) string {
	keys := map[string]Key{
		"auto": KeyRuntimeSkillVisibilityAuto, "name-only": KeyRuntimeSkillVisibilityNameOnly,
		"manual-only": KeyRuntimeSkillVisibilityManualOnly, "off": KeyRuntimeSkillVisibilityOff,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeSkillScopeLabel(lang Language, code string) string {
	keys := map[string]Key{
		"default": KeyRuntimeSkillScopeDefault, "frontmatter": KeyRuntimeSkillScopeFrontmatter,
		"user": KeyRuntimeSkillScopeUser, "project": KeyRuntimeSkillScopeProject,
		"session": KeyRuntimeSkillScopeSession, "managed": KeyRuntimeSkillScopeManaged,
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeSkillSourceLabel(lang Language, code string) string {
	keys := map[string]Key{
		"project": KeyRuntimeSkillSourceProject, "user": KeyRuntimeSkillSourceUser,
		"managed": KeyRuntimeSkillSourceManaged, "plugin": KeyRuntimeSkillSourcePlugin,
		"bundled": KeyRuntimeSkillSourceBundled,
	}
	if code == "mcp" {
		return "MCP"
	}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}

func RuntimeSkillContextLabel(lang Language, code string) string {
	keys := map[string]Key{"inline": KeyRuntimeSkillContextInline, "fork": KeyRuntimeSkillContextFork}
	if key, ok := keys[code]; ok {
		return Text(lang, key)
	}
	return code
}
