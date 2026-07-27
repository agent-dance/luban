package i18n

// Semantic copy emitted before or after the interactive renderer is active.
// Runtime identifiers and underlying errors are supplied as format values.
const (
	KeyStartupUnsupportedOutput      Key = "startup.output.unsupported"
	KeyStartupFatal                  Key = "startup.fatal"
	KeyStartupWarning                Key = "startup.warning"
	KeyStartupCredentialStoreWarning Key = "startup.credential_store.warning"
	KeyStartupOAuthStoreWarning      Key = "startup.oauth_store.warning"
	KeyStartupWorkingDirectoryFatal  Key = "startup.working_directory.fatal"
	KeyStartupNoActiveModel          Key = "startup.model.no_active"
	KeyStartupSessionFatal           Key = "startup.session.fatal"
	KeyStartupSandboxUnavailable     Key = "startup.sandbox.unavailable"
	KeyStartupSafetyDenied           Key = "startup.safety.denied"
	KeyStartupShutdownWarning        Key = "startup.shutdown.warning"
	KeyStartupShutdownSchedule       Key = "startup.shutdown.schedule"
	KeyStartupShutdownEngine         Key = "startup.shutdown.engine"
	KeyStartupShutdownBackground     Key = "startup.shutdown.background"
	KeyStartupShutdownMCP            Key = "startup.shutdown.mcp"
	KeyStartupShutdownLSP            Key = "startup.shutdown.lsp"
	KeyStartupShutdownProvider       Key = "startup.shutdown.provider"
	KeyStartupShutdownDebugFile      Key = "startup.shutdown.debug_file"
	KeyStartupSDKError               Key = "startup.sdk.error"
	KeyStartupResumeWarning          Key = "startup.session.resume_warning"
	KeyStartupResumed                Key = "startup.session.resumed"
	KeyStartupProviderMismatch       Key = "startup.session.provider_mismatch"
	KeyStartupScreenReaderError      Key = "startup.screen_reader.error"
	KeyStartupTUIError               Key = "startup.tui.error"
	KeyPrintQueryRequired            Key = "print.query.required"
	KeyStartupResolveSession         Key = "startup.session.resolve_error"
	KeyStartupLatestSessionWarning   Key = "startup.session.latest_warning"
	KeyStartupLoadSessionMetadata    Key = "startup.session.metadata_error"
	KeyStartupResolveLatestSession   Key = "startup.session.latest_error"
)

func init() {
	semanticTranslations[KeyStartupUnsupportedOutput] = startupCopy(
		"unsupported output format %q: expected text, json, or stream-json",
		"不支持输出格式 %q：应为 text、json 或 stream-json",
		"Nicht unterstütztes Ausgabeformat %q: Erwartet wird text, json oder stream-json",
		"未対応の出力形式 %q です。text、json、stream-json のいずれかを指定してください",
		"지원하지 않는 출력 형식 %q입니다. text, json 또는 stream-json을 사용하세요",
		"Неподдерживаемый формат вывода %q: ожидается text, json или stream-json",
	)
	semanticTranslations[KeyStartupFatal] = startupCopy("Fatal: %v\n", "致命错误：%v\n", "Schwerwiegender Fehler: %v\n", "致命的なエラー: %v\n", "치명적 오류: %v\n", "Критическая ошибка: %v\n")
	semanticTranslations[KeyStartupWarning] = startupCopy("Warning: %v\n", "警告：%v\n", "Warnung: %v\n", "警告: %v\n", "경고: %v\n", "Предупреждение: %v\n")
	semanticTranslations[KeyStartupCredentialStoreWarning] = startupCopy(
		"Warning: credential store unavailable: %v\n",
		"警告：凭据存储不可用：%v\n",
		"Warnung: Anmeldedatenspeicher nicht verfügbar: %v\n",
		"警告: 認証情報ストアを利用できません: %v\n",
		"경고: 자격 증명 저장소를 사용할 수 없습니다: %v\n",
		"Предупреждение: хранилище учётных данных недоступно: %v\n",
	)
	semanticTranslations[KeyStartupOAuthStoreWarning] = startupCopy(
		"Warning: OAuth store unavailable: %v\n",
		"警告：OAuth 存储不可用：%v\n",
		"Warnung: OAuth-Speicher nicht verfügbar: %v\n",
		"警告: OAuth ストアを利用できません: %v\n",
		"경고: OAuth 저장소를 사용할 수 없습니다: %v\n",
		"Предупреждение: хранилище OAuth недоступно: %v\n",
	)
	semanticTranslations[KeyStartupWorkingDirectoryFatal] = startupCopy(
		"Fatal: cannot determine working directory: %v\n",
		"致命错误：无法确定工作目录：%v\n",
		"Schwerwiegender Fehler: Arbeitsverzeichnis kann nicht ermittelt werden: %v\n",
		"致命的なエラー: 作業ディレクトリを特定できません: %v\n",
		"치명적 오류: 작업 디렉터리를 확인할 수 없습니다: %v\n",
		"Критическая ошибка: не удалось определить рабочий каталог: %v\n",
	)
	semanticTranslations[KeyStartupNoActiveModel] = startupCopy(
		"Starting interactive mode without an active model. Connect first with /connect %s --oauth or /connect %s.\n",
		"将在没有活动模型的情况下启动交互模式。请先使用 /connect %s --oauth 或 /connect %s 连接。\n",
		"Der interaktive Modus wird ohne aktives Modell gestartet. Stelle zuerst mit /connect %s --oauth oder /connect %s eine Verbindung her.\n",
		"有効なモデルがない状態で対話モードを開始します。先に /connect %s --oauth または /connect %s で接続してください。\n",
		"활성 모델 없이 대화형 모드를 시작합니다. 먼저 /connect %s --oauth 또는 /connect %s로 연결하세요.\n",
		"Интерактивный режим запускается без активной модели. Сначала подключитесь командой /connect %s --oauth или /connect %s.\n",
	)
	semanticTranslations[KeyStartupSessionFatal] = startupCopy(
		"Fatal: session startup failed: %v\n",
		"致命错误：会话启动失败：%v\n",
		"Schwerwiegender Fehler: Sitzung konnte nicht gestartet werden: %v\n",
		"致命的なエラー: セッションを開始できませんでした: %v\n",
		"치명적 오류: 세션을 시작하지 못했습니다: %v\n",
		"Критическая ошибка: не удалось запустить сеанс: %v\n",
	)
	semanticTranslations[KeyStartupSandboxUnavailable] = startupCopy(
		"Warning: --sandbox was requested, but this platform has no available sandbox backend.\n",
		"警告：已请求 --sandbox，但当前平台没有可用的 sandbox 后端。\n",
		"Warnung: --sandbox wurde angefordert, aber auf dieser Plattform ist kein Sandbox-Backend verfügbar.\n",
		"警告: --sandbox が指定されましたが、このプラットフォームでは sandbox バックエンドを利用できません。\n",
		"경고: --sandbox가 요청되었지만 이 플랫폼에서는 사용할 수 있는 sandbox 백엔드가 없습니다.\n",
		"Предупреждение: запрошен --sandbox, но на этой платформе нет доступного sandbox-бэкенда.\n",
	)
	semanticTranslations[KeyStartupSafetyDenied] = startupCopy(
		"Safety: denied %s — %s\n",
		"安全策略：已拒绝 %s — %s\n",
		"Sicherheit: %s abgelehnt — %s\n",
		"安全機能: %s を拒否しました — %s\n",
		"안전 정책: %s 거부됨 — %s\n",
		"Безопасность: %s отклонено — %s\n",
	)
	semanticTranslations[KeyStartupShutdownWarning] = startupCopy(
		"Warning: shutdown failed: %v\n",
		"警告：关闭失败：%v\n",
		"Warnung: Herunterfahren fehlgeschlagen: %v\n",
		"警告: 終了処理に失敗しました: %v\n",
		"경고: 종료하지 못했습니다: %v\n",
		"Предупреждение: не удалось завершить работу: %v\n",
	)
	semanticTranslations[KeyStartupShutdownSchedule] = startupCopy(
		"the schedule service did not stop cleanly",
		"计划任务服务未能正常停止",
		"Der Zeitplandienst wurde nicht ordnungsgemäß beendet",
		"スケジュールサービスを正常に停止できませんでした",
		"예약 서비스가 정상적으로 종료되지 않았습니다",
		"службу расписания не удалось корректно остановить",
	)
	semanticTranslations[KeyStartupShutdownEngine] = startupCopy(
		"the conversation runtime did not stop cleanly",
		"会话运行时未能正常停止",
		"Die Konversationslaufzeit wurde nicht ordnungsgemäß beendet",
		"会話ランタイムを正常に停止できませんでした",
		"대화 런타임이 정상적으로 종료되지 않았습니다",
		"среду выполнения диалога не удалось корректно остановить",
	)
	semanticTranslations[KeyStartupShutdownBackground] = startupCopy(
		"background tasks did not stop cleanly",
		"后台任务未能正常停止",
		"Hintergrundaufgaben wurden nicht ordnungsgemäß beendet",
		"バックグラウンドタスクを正常に停止できませんでした",
		"백그라운드 작업이 정상적으로 종료되지 않았습니다",
		"фоновые задачи не удалось корректно остановить",
	)
	semanticTranslations[KeyStartupShutdownMCP] = startupCopy(
		"MCP services did not stop cleanly",
		"MCP 服务未能正常停止",
		"Die MCP-Dienste wurden nicht ordnungsgemäß beendet",
		"MCP サービスを正常に停止できませんでした",
		"MCP 서비스가 정상적으로 종료되지 않았습니다",
		"службы MCP не удалось корректно остановить",
	)
	semanticTranslations[KeyStartupShutdownLSP] = startupCopy(
		"language server processes did not stop cleanly",
		"语言服务器进程未能正常停止",
		"Die Sprachserverprozesse wurden nicht ordnungsgemäß beendet",
		"言語サーバープロセスを正常に停止できませんでした",
		"언어 서버 프로세스가 정상적으로 종료되지 않았습니다",
		"процессы языковых серверов не удалось корректно остановить",
	)
	semanticTranslations[KeyStartupShutdownProvider] = startupCopy(
		"the provider transport did not close cleanly",
		"Provider 传输未能正常关闭",
		"Der Provider-Transport wurde nicht ordnungsgemäß geschlossen",
		"Provider トランスポートを正常に閉じることができませんでした",
		"Provider 전송이 정상적으로 닫히지 않았습니다",
		"транспорт провайдера не удалось корректно закрыть",
	)
	semanticTranslations[KeyStartupShutdownDebugFile] = startupCopy(
		"the debug output file did not close cleanly",
		"调试输出文件未能正常关闭",
		"Die Debug-Ausgabedatei wurde nicht ordnungsgemäß geschlossen",
		"デバッグ出力ファイルを正常に閉じることができませんでした",
		"디버그 출력 파일이 정상적으로 닫히지 않았습니다",
		"файл отладочного вывода не удалось корректно закрыть",
	)
	semanticTranslations[KeyStartupSDKError] = startupCopy("SDK error: %v\n", "SDK 错误：%v\n", "SDK-Fehler: %v\n", "SDK エラー: %v\n", "SDK 오류: %v\n", "Ошибка SDK: %v\n")
	semanticTranslations[KeyStartupResumeWarning] = startupCopy(
		"Warning: could not resume session %s: %v\n",
		"警告：无法恢复会话 %s：%v\n",
		"Warnung: Sitzung %s konnte nicht fortgesetzt werden: %v\n",
		"警告: セッション %s を再開できませんでした: %v\n",
		"경고: 세션 %s을(를) 재개할 수 없습니다: %v\n",
		"Предупреждение: не удалось возобновить сеанс %s: %v\n",
	)
	semanticTranslations[KeyStartupResumed] = startupCopy(
		"Resumed session: %s (%d messages)\n",
		"已恢复会话：%s（%d 条消息）\n",
		"Sitzung fortgesetzt: %s (%d Nachrichten)\n",
		"セッションを再開しました: %s（%d 件のメッセージ）\n",
		"세션 재개됨: %s (메시지 %d개)\n",
		"Сеанс возобновлён: %s (сообщений: %d)\n",
	)
	semanticTranslations[KeyStartupProviderMismatch] = startupCopy(
		"Session was created with %s/%s, but the current provider is %s/%s.\n",
		"该会话创建时使用 %s/%s，但当前 Provider 为 %s/%s。\n",
		"Die Sitzung wurde mit %s/%s erstellt, aktuell wird jedoch %s/%s verwendet.\n",
		"このセッションは %s/%s で作成されましたが、現在のプロバイダーは %s/%s です。\n",
		"이 세션은 %s/%s로 생성되었지만 현재 Provider는 %s/%s입니다.\n",
		"Сеанс был создан с %s/%s, но текущий провайдер — %s/%s.\n",
	)
	semanticTranslations[KeyStartupScreenReaderError] = startupCopy(
		"Screen reader error: %v\n",
		"屏幕阅读器错误：%v\n",
		"Screenreader-Fehler: %v\n",
		"スクリーンリーダーエラー: %v\n",
		"스크린 리더 오류: %v\n",
		"Ошибка программы чтения с экрана: %v\n",
	)
	semanticTranslations[KeyStartupTUIError] = startupCopy("TUI error: %v\n", "TUI 错误：%v\n", "TUI-Fehler: %v\n", "TUI エラー: %v\n", "TUI 오류: %v\n", "Ошибка TUI: %v\n")
	semanticTranslations[KeyPrintQueryRequired] = startupCopy("Error: -p requires a query argument\n", "错误：-p 需要查询参数\n", "Fehler: -p benötigt eine Anfrage\n", "エラー: -p にはクエリ引数が必要です\n", "오류: -p에는 질의 인수가 필요합니다\n", "Ошибка: для -p требуется аргумент запроса\n")
	semanticTranslations[KeyStartupResolveSession] = startupCopy("Could not resolve session %s.", "无法解析会话 %s。", "Sitzung %s konnte nicht aufgelöst werden.", "セッション %s を解決できませんでした。", "세션 %s을(를) 확인할 수 없습니다.", "Не удалось найти сеанс %s.")
	semanticTranslations[KeyStartupLatestSessionWarning] = startupCopy("Warning: could not find the latest session: %v\n", "警告：找不到最近的会话：%v\n", "Warnung: Die letzte Sitzung wurde nicht gefunden: %v\n", "警告: 直近のセッションが見つかりませんでした: %v\n", "경고: 가장 최근 세션을 찾을 수 없습니다: %v\n", "Предупреждение: не удалось найти последний сеанс: %v\n")
	semanticTranslations[KeyStartupLoadSessionMetadata] = startupCopy("Could not load metadata for session %s: %v", "无法加载会话 %s 的元数据：%v", "Metadaten der Sitzung %s konnten nicht geladen werden: %v", "セッション %s のメタデータを読み込めませんでした: %v", "세션 %s의 메타데이터를 불러올 수 없습니다: %v", "Не удалось загрузить метаданные сеанса %s: %v")
	semanticTranslations[KeyStartupResolveLatestSession] = startupCopy("Could not resolve the latest session: %v", "无法解析最近的会话：%v", "Die letzte Sitzung konnte nicht aufgelöst werden: %v", "直近のセッションを解決できませんでした: %v", "가장 최근 세션을 확인할 수 없습니다: %v", "Не удалось найти последний сеанс: %v")
}

func startupCopy(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
