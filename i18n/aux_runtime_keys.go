package i18n

const (
	KeyAuxCompactNotEnoughMessages Key = "aux.compact.not_enough_messages"
	KeyAuxCompactInterrupted       Key = "aux.compact.interrupted"
	KeyAuxCompactConversationLong  Key = "aux.compact.conversation_too_long"
	KeyAuxCompactSummaryMissing    Key = "aux.compact.summary_missing"
	KeyAuxCompactCancelled         Key = "aux.compact.cancelled"
	KeyAuxCompactFailed            Key = "aux.compact.failed"
	KeyAuxCompactEmptyToolResult   Key = "aux.compact.empty_tool_result"
	KeyAuxCompactOutputTooLarge    Key = "aux.compact.output_too_large"
	KeyAuxCompactPreview           Key = "aux.compact.preview"
	KeyAuxCompactBytes             Key = "aux.compact.bytes"
	KeyAuxCompactTruncated         Key = "aux.compact.truncated"
	KeyAuxCompactBudgetTruncated   Key = "aux.compact.budget_truncated"
	KeyAuxCompactInvalidDirection  Key = "aux.compact.invalid_direction"
	KeyAuxCompactEmptyHistory      Key = "aux.compact.empty_history"
	KeyAuxCompactInvalidPivot      Key = "aux.compact.invalid_pivot"
	KeyAuxCompactNothingBefore     Key = "aux.compact.nothing_before"
	KeyAuxCompactNothingAfter      Key = "aux.compact.nothing_after"
	KeyAuxCompactPreserveNone      Key = "aux.compact.preserve_none"
	KeyAuxCompactImageRemoved      Key = "aux.compact.image_removed"
	KeyAuxCompactDocumentRemoved   Key = "aux.compact.document_removed"

	KeyAuxClipboardUnsupported       Key = "aux.clipboard.unsupported"
	KeyAuxClipboardCreateTemp        Key = "aux.clipboard.create_temp_failed"
	KeyAuxClipboardReadTemp          Key = "aux.clipboard.read_temp_failed"
	KeyAuxClipboardMissingReference  Key = "aux.clipboard.missing_file_reference"
	KeyAuxClipboardReferenceNotImage Key = "aux.clipboard.reference_not_image"
	KeyAuxClipboardReadImage         Key = "aux.clipboard.read_image_failed"
	KeyAuxClipboardLinuxUnavailable  Key = "aux.clipboard.linux_unavailable"
	KeyAuxClipboardPowerShellFailed  Key = "aux.clipboard.powershell_failed"

	KeyAuxHookBlockedContinuation   Key = "aux.hook.blocked_continuation"
	KeyAuxHookPreventedContinuation Key = "aux.hook.prevented_continuation"
	KeyAuxHookExecutionFailed       Key = "aux.hook.execution_failed"
	KeyAuxToolHookBlocked           Key = "aux.hook.tool_blocked"
	KeyAuxToolHookPrevented         Key = "aux.hook.tool_prevented"
	KeyAuxHookNamedFeedback         Key = "aux.hook.named_feedback"
	KeyAuxHookNamedBlocked          Key = "aux.hook.named_blocked"
	KeyAuxPostSamplingBlocked       Key = "aux.hook.post_sampling_blocked"
	KeyAuxPostSamplingBlockedReason Key = "aux.hook.post_sampling_blocked_reason"

	KeyAuxSessionNoSessions Key = "aux.session.none"
	KeyAuxSessionDeleted    Key = "aux.session.deleted"
	KeyAuxSessionNotFound   Key = "aux.session.not_found"
	KeyAuxSessionAmbiguous  Key = "aux.session.ambiguous"
	KeyAuxSessionFailed     Key = "aux.session.failed"

	KeyAuxEngineSessionNotFound Key = "aux.engine.session_not_found"
	KeyAuxEngineSessionDeleted  Key = "aux.engine.session_deleted"
	KeyAuxEngineShutdown        Key = "aux.engine.shutdown"
	KeyAuxEngineNoProvider      Key = "aux.engine.no_provider"

	KeyAuxSwarmTeamNotFound  Key = "aux.swarm.team_not_found"
	KeyAuxSwarmInvalidName   Key = "aux.swarm.invalid_name"
	KeyAuxSwarmMailboxFailed Key = "aux.swarm.mailbox_failed"
	KeyAuxSwarmFailed        Key = "aux.swarm.failed"
	KeyAuxSwarmPaneTitle     Key = "aux.swarm.debug.pane_title"
	KeyAuxSwarmPaneBorder    Key = "aux.swarm.debug.pane_border"
	KeyAuxSwarmLayout        Key = "aux.swarm.debug.layout"

	KeyAuxSkillNotFound         Key = "aux.skills.not_found"
	KeyAuxSkillRevisionConflict Key = "aux.skills.revision_conflict"
	KeyAuxSkillManagedReadOnly  Key = "aux.skills.managed_read_only"
	KeyAuxSkillInvalidScope     Key = "aux.skills.invalid_scope"
	KeyAuxSkillInvalidSession   Key = "aux.skills.invalid_session"
	KeyAuxSkillFailed           Key = "aux.skills.failed"
	KeyAuxMCPPromptDescription  Key = "aux.skills.mcp_prompt_description"

	KeyAuxMCPToolError Key = "aux.mcp.tool_error"
)

func init() {
	addAux(KeyAuxCompactNotEnoughMessages, "Not enough messages to compact.", "消息数量不足，无法压缩。", "Es sind nicht genügend Nachrichten zum Komprimieren vorhanden.", "圧縮するメッセージが足りません。", "압축할 메시지가 충분하지 않습니다.", "Недостаточно сообщений для сжатия.")
	addAux(KeyAuxCompactInterrupted, "Compaction interrupted · This may be due to network issues — please try again.", "压缩已中断，可能是网络问题导致的。请重试。", "Die Komprimierung wurde möglicherweise wegen eines Netzwerkproblems unterbrochen. Versuche es erneut.", "ネットワークの問題により圧縮が中断された可能性があります。もう一度お試しください。", "네트워크 문제로 압축이 중단되었을 수 있습니다. 다시 시도하세요.", "Сжатие было прервано, возможно из-за проблем с сетью. Повторите попытку.")
	addAux(KeyAuxCompactConversationLong, "Conversation too long. Press esc twice to go up a few messages and try again.", "对话过长。请按两次 Esc，退回几条消息后重试。", "Die Unterhaltung ist zu lang. Drücke zweimal Esc, gehe einige Nachrichten zurück und versuche es erneut.", "会話が長すぎます。Esc を2回押し、数件前のメッセージに戻ってから再試行してください。", "대화가 너무 깁니다. Esc를 두 번 누르고 몇 개의 메시지 이전으로 돌아가 다시 시도하세요.", "Диалог слишком длинный. Дважды нажмите Esc, вернитесь на несколько сообщений назад и повторите попытку.")
	addAux(KeyAuxCompactSummaryMissing, "Failed to generate conversation summary - response did not contain valid text content", "响应中没有有效文本，无法生成对话摘要。", "Es konnte keine Zusammenfassung erstellt werden, da die Antwort keinen gültigen Text enthielt.", "応答に有効なテキストがなかったため、会話の要約を生成できませんでした。", "응답에 유효한 텍스트가 없어 대화 요약을 생성할 수 없습니다.", "Не удалось создать сводку диалога: ответ не содержит корректного текста.")
	addAux(KeyAuxCompactCancelled, "Compaction canceled.", "已取消压缩。", "Komprimierung abgebrochen.", "圧縮をキャンセルしました。", "압축이 취소되었습니다.", "Сжатие отменено.")
	addAux(KeyAuxCompactFailed, "Error during compaction: %v", "压缩失败：%v", "Komprimierung fehlgeschlagen: %v", "圧縮に失敗しました: %v", "압축 실패: %v", "Ошибка сжатия: %v")
	addAux(KeyAuxCompactEmptyToolResult, "(%s completed with no output)", "（%s 已完成，无输出）", "(%s abgeschlossen, keine Ausgabe)", "（%s は完了しましたが、出力はありません）", "(%s 완료, 출력 없음)", "(%s завершён без вывода)")
	addAux(KeyAuxCompactOutputTooLarge, "Output too large (%s). Full output saved to: %s\n\n", "输出过大（%s）。完整输出已保存至：%s\n\n", "Die Ausgabe ist zu groß (%s). Die vollständige Ausgabe wurde gespeichert unter: %s\n\n", "出力が大きすぎます（%s）。完全な出力の保存先: %s\n\n", "출력이 너무 큽니다(%s). 전체 출력 저장 위치: %s\n\n", "Вывод слишком велик (%s). Полный вывод сохранён в: %s\n\n")
	addAux(KeyAuxCompactPreview, "Preview (first %s):\n", "预览（前 %s）：\n", "Vorschau (erste %s):\n", "プレビュー（先頭 %s）:\n", "미리보기(처음 %s):\n", "Предпросмотр (первые %s):\n")
	addAux(KeyAuxCompactBytes, "%d bytes", "%d 字节", "%d Byte", "%d バイト", "%d바이트", "%d байт")
	addAux(KeyAuxCompactTruncated, "\n\n... (truncated, %d chars total)", "\n\n……（已截断，共 %d 个字符）", "\n\n… (gekürzt; insgesamt %d Zeichen)", "\n\n…（切り詰めました。全 %d 文字）", "\n\n… (잘림, 전체 %d자)", "\n\n… (усечено; всего %d символов)")
	addAux(KeyAuxCompactBudgetTruncated, "%s\n\n... (truncated by per-message budget, %d chars total)", "%s\n\n……（因单条消息预算限制而截断，共 %d 个字符）", "%s\n\n… (wegen des Budgets pro Nachricht gekürzt; insgesamt %d Zeichen)", "%s\n\n…（メッセージ単位の上限により切り詰めました。全 %d 文字）", "%s\n\n… (메시지별 한도로 인해 잘림, 전체 %d자)", "%s\n\n… (усечено из-за лимита на сообщение; всего %d символов)")
	addAux(KeyAuxCompactInvalidDirection, "Invalid partial-compaction direction: %s", "无效的部分压缩方向：%s", "Ungültige Richtung für die partielle Komprimierung: %s", "部分圧縮の方向が無効です: %s", "부분 압축 방향이 올바르지 않습니다: %s", "Недопустимое направление частичного сжатия: %s")
	addAux(KeyAuxCompactEmptyHistory, "There is no conversation history to compact.", "没有可压缩的对话历史。", "Es gibt keinen Unterhaltungsverlauf zum Komprimieren.", "圧縮する会話履歴がありません。", "압축할 대화 기록이 없습니다.", "Нет истории диалога для сжатия.")
	addAux(KeyAuxCompactInvalidPivot, "The selected compaction point %d is invalid for %d messages.", "所选压缩位置 %d 对 %d 条消息无效。", "Der gewählte Komprimierungspunkt %d ist für %d Nachrichten ungültig.", "選択した圧縮位置 %d は %d 件のメッセージに対して無効です。", "선택한 압축 지점 %d은(는) 메시지 %d개에 유효하지 않습니다.", "Выбранная точка сжатия %d недопустима для %d сообщений.")
	addAux(KeyAuxCompactNothingBefore, "There are no messages before the selected message to summarize.", "所选消息之前没有可摘要的内容。", "Vor der ausgewählten Nachricht gibt es nichts zusammenzufassen.", "選択したメッセージより前に要約する内容がありません。", "선택한 메시지 앞에 요약할 내용이 없습니다.", "До выбранного сообщения нет данных для сводки.")
	addAux(KeyAuxCompactNothingAfter, "There are no messages after the selected message to summarize.", "所选消息之后没有可摘要的内容。", "Nach der ausgewählten Nachricht gibt es nichts zusammenzufassen.", "選択したメッセージより後に要約する内容がありません。", "선택한 메시지 뒤에 요약할 내용이 없습니다.", "После выбранного сообщения нет данных для сводки.")
	addAux(KeyAuxCompactPreserveNone, "Partial compaction would preserve no messages.", "部分压缩将不会保留任何消息。", "Bei der partiellen Komprimierung würden keine Nachrichten erhalten bleiben.", "部分圧縮ではメッセージが1件も保持されません。", "부분 압축 시 보존되는 메시지가 없습니다.", "При частичном сжатии не останется ни одного сообщения.")
	addAux(KeyAuxCompactImageRemoved, "[image removed: media exceeded provider size limit]", "[图片已移除：媒体超过 Provider 大小限制]", "[Bild entfernt: Medium überschritt das Größenlimit des Providers]", "[画像を削除しました: メディアが Provider のサイズ上限を超えています]", "[이미지 제거됨: 미디어가 Provider 크기 제한을 초과함]", "[изображение удалено: размер медиа превышает лимит Provider]")
	addAux(KeyAuxCompactDocumentRemoved, "[document removed: media exceeded provider size limit]", "[文档已移除：媒体超过 Provider 大小限制]", "[Dokument entfernt: Medium überschritt das Größenlimit des Providers]", "[ドキュメントを削除しました: メディアが Provider のサイズ上限を超えています]", "[문서 제거됨: 미디어가 Provider 크기 제한을 초과함]", "[документ удалён: размер медиа превышает лимит Provider]")

	addAux(KeyAuxClipboardUnsupported, "Clipboard image paste is not supported on %s.", "%s 暂不支持粘贴剪贴板图片。", "Das Einfügen von Bildern aus der Zwischenablage wird unter %s nicht unterstützt.", "%s ではクリップボード画像の貼り付けに対応していません。", "%s에서는 클립보드 이미지 붙여넣기를 지원하지 않습니다.", "Вставка изображений из буфера обмена не поддерживается в %s.")
	addAux(KeyAuxClipboardCreateTemp, "Could not create a temporary clipboard file: %v", "无法创建剪贴板临时文件：%v", "Temporäre Zwischenablagedatei konnte nicht erstellt werden: %v", "クリップボード用の一時ファイルを作成できませんでした: %v", "클립보드 임시 파일을 만들 수 없습니다: %v", "Не удалось создать временный файл буфера обмена: %v")
	addAux(KeyAuxClipboardReadTemp, "Could not read the temporary clipboard file: %v", "无法读取剪贴板临时文件：%v", "Temporäre Zwischenablagedatei konnte nicht gelesen werden: %v", "クリップボード用の一時ファイルを読み込めませんでした: %v", "클립보드 임시 파일을 읽을 수 없습니다: %v", "Не удалось прочитать временный файл буфера обмена: %v")
	addAux(KeyAuxClipboardMissingReference, "The clipboard does not contain a file reference.", "剪贴板中没有文件引用。", "Die Zwischenablage enthält keinen Dateiverweis.", "クリップボードにファイル参照がありません。", "클립보드에 파일 참조가 없습니다.", "В буфере обмена нет ссылки на файл.")
	addAux(KeyAuxClipboardReferenceNotImage, "The clipboard file is not a supported image: %s", "剪贴板中的文件不是受支持的图片：%s", "Die Datei aus der Zwischenablage ist kein unterstütztes Bild: %s", "クリップボードのファイルは対応画像ではありません: %s", "클립보드 파일이 지원되는 이미지가 아닙니다: %s", "Файл из буфера обмена не является поддерживаемым изображением: %s")
	addAux(KeyAuxClipboardReadImage, "Could not read clipboard image %s: %v", "无法读取剪贴板图片 %s：%v", "Zwischenablagebild %s konnte nicht gelesen werden: %v", "クリップボード画像 %s を読み込めませんでした: %v", "클립보드 이미지 %s을(를) 읽을 수 없습니다: %v", "Не удалось прочитать изображение из буфера %s: %v")
	addAux(KeyAuxClipboardLinuxUnavailable, "Could not read a clipboard image on Linux (tried wl-paste and xclip).", "无法在 Linux 上读取剪贴板图片（已尝试 wl-paste 和 xclip）。", "Unter Linux konnte kein Zwischenablagebild gelesen werden (wl-paste und xclip wurden versucht).", "Linux でクリップボード画像を読み込めませんでした（wl-paste と xclip を試行済み）。", "Linux에서 클립보드 이미지를 읽을 수 없습니다(wl-paste와 xclip 시도함).", "Не удалось прочитать изображение из буфера в Linux (проверены wl-paste и xclip).")
	addAux(KeyAuxClipboardPowerShellFailed, "PowerShell could not read the clipboard: %v\n%s", "PowerShell 无法读取剪贴板：%v\n%s", "PowerShell konnte die Zwischenablage nicht lesen: %v\n%s", "PowerShell でクリップボードを読み込めませんでした: %v\n%s", "PowerShell에서 클립보드를 읽지 못했습니다: %v\n%s", "PowerShell не удалось прочитать буфер обмена: %v\n%s")

	addAux(KeyAuxHookBlockedContinuation, "hook blocked continuation", "Hook 阻止了继续执行", "Hook hat die Fortsetzung blockiert", "Hook が続行をブロックしました", "Hook이 계속 진행을 차단했습니다", "Hook заблокировал продолжение")
	addAux(KeyAuxHookPreventedContinuation, "hook prevented continuation", "Hook 阻止了继续执行", "Hook hat die Fortsetzung verhindert", "Hook により続行できません", "Hook으로 인해 계속 진행할 수 없습니다", "Hook предотвратил продолжение")
	addAux(KeyAuxHookExecutionFailed, "hook exited with an error", "Hook 执行出错并退出", "Hook wurde mit einem Fehler beendet", "Hook がエラーで終了しました", "Hook이 오류와 함께 종료되었습니다", "Hook завершился с ошибкой")
	addAux(KeyAuxToolHookBlocked, "tool hook blocked execution", "工具 Hook 阻止了执行", "Tool-Hook hat die Ausführung blockiert", "ツール Hook が実行をブロックしました", "도구 Hook이 실행을 차단했습니다", "Hook инструмента заблокировал выполнение")
	addAux(KeyAuxToolHookPrevented, "tool hook prevented continuation", "工具 Hook 阻止了继续执行", "Tool-Hook hat die Fortsetzung verhindert", "ツール Hook により続行できません", "도구 Hook으로 인해 계속 진행할 수 없습니다", "Hook инструмента предотвратил продолжение")
	addAux(KeyAuxHookNamedFeedback, "%s hook feedback", "%s Hook 反馈", "%s-Hook-Rückmeldung", "%s Hook のフィードバック", "%s Hook 피드백", "Обратная связь Hook %s")
	addAux(KeyAuxHookNamedBlocked, "%s hook blocked continuation", "%s Hook 阻止了继续执行", "%s-Hook hat die Fortsetzung blockiert", "%s Hook が続行をブロックしました", "%s Hook이 계속 진행을 차단했습니다", "Hook %s заблокировал продолжение")
	addAux(KeyAuxPostSamplingBlocked, "post-sampling hook blocked continuation", "PostSampling Hook 阻止了继续执行", "PostSampling-Hook hat die Fortsetzung blockiert", "PostSampling Hook が続行をブロックしました", "PostSampling Hook이 계속 진행을 차단했습니다", "Hook PostSampling заблокировал продолжение")
	addAux(KeyAuxPostSamplingBlockedReason, "post-sampling hook blocked continuation: %s", "PostSampling Hook 阻止了继续执行：%s", "PostSampling-Hook hat die Fortsetzung blockiert: %s", "PostSampling Hook が続行をブロックしました: %s", "PostSampling Hook이 계속 진행을 차단했습니다: %s", "Hook PostSampling заблокировал продолжение: %s")

	addAux(KeyAuxSessionNoSessions, "No saved sessions were found.", "未找到已保存的会话。", "Keine gespeicherten Sitzungen gefunden.", "保存済みのセッションがありません。", "저장된 세션을 찾을 수 없습니다.", "Сохранённые сеансы не найдены.")
	addAux(KeyAuxSessionDeleted, "This session's history has been deleted.", "此会话的历史记录已被删除。", "Der Verlauf dieser Sitzung wurde gelöscht.", "このセッションの履歴は削除されています。", "이 세션의 기록이 삭제되었습니다.", "История этого сеанса удалена.")
	addAux(KeyAuxSessionNotFound, "The requested session was not found.", "未找到请求的会话。", "Die angeforderte Sitzung wurde nicht gefunden.", "指定されたセッションが見つかりません。", "요청한 세션을 찾을 수 없습니다.", "Запрошенный сеанс не найден.")
	addAux(KeyAuxSessionAmbiguous, "That session ID exists in more than one project. Select it by title or from the current project.", "该会话 ID 存在于多个项目中。请按标题选择，或从当前项目中选择。", "Diese Sitzungs-ID existiert in mehreren Projekten. Wähle sie über den Titel oder im aktuellen Projekt aus.", "そのセッション ID は複数のプロジェクトに存在します。タイトルまたは現在のプロジェクトから選択してください。", "해당 세션 ID가 여러 프로젝트에 있습니다. 제목이나 현재 프로젝트에서 선택하세요.", "Этот ID сеанса встречается в нескольких проектах. Выберите сеанс по названию или в текущем проекте.")
	addAux(KeyAuxSessionFailed, "The session operation failed.", "会话操作失败。", "Der Sitzungsvorgang ist fehlgeschlagen.", "セッション操作に失敗しました。", "세션 작업에 실패했습니다.", "Операция с сеансом завершилась ошибкой.")

	addAux(KeyAuxEngineSessionNotFound, "The runtime could not find this session.", "运行时未找到此会话。", "Die Laufzeit konnte diese Sitzung nicht finden.", "ランタイムでこのセッションが見つかりません。", "런타임에서 이 세션을 찾을 수 없습니다.", "Среда выполнения не нашла этот сеанс.")
	addAux(KeyAuxEngineSessionDeleted, "The runtime cannot use a session whose history was deleted.", "运行时无法使用历史记录已删除的会话。", "Die Laufzeit kann keine Sitzung mit gelöschtem Verlauf verwenden.", "履歴が削除されたセッションはランタイムで使用できません。", "런타임은 기록이 삭제된 세션을 사용할 수 없습니다.", "Среда выполнения не может использовать сеанс с удалённой историей.")
	addAux(KeyAuxEngineShutdown, "The runtime has already shut down.", "运行时已关闭。", "Die Laufzeit wurde bereits beendet.", "ランタイムはすでに終了しています。", "런타임이 이미 종료되었습니다.", "Среда выполнения уже остановлена.")
	addAux(KeyAuxEngineNoProvider, "No Provider is configured.", "尚未配置 Provider。", "Kein Provider ist konfiguriert.", "Provider が設定されていません。", "Provider가 구성되어 있지 않습니다.", "Provider не настроен.")

	addAux(KeyAuxSwarmTeamNotFound, "The requested team was not found.", "未找到请求的团队。", "Das angeforderte Team wurde nicht gefunden.", "指定されたチームが見つかりません。", "요청한 팀을 찾을 수 없습니다.", "Запрошенная команда не найдена.")
	addAux(KeyAuxSwarmInvalidName, "The team or agent name is invalid. Use letters, numbers, underscores, or hyphens.", "团队或 Agent 名称无效。请使用字母、数字、下划线或连字符。", "Der Team- oder Agentenname ist ungültig. Verwende Buchstaben, Zahlen, Unterstriche oder Bindestriche.", "チーム名または Agent 名が無効です。英数字、アンダースコア、ハイフンを使用してください。", "팀 또는 Agent 이름이 올바르지 않습니다. 문자, 숫자, 밑줄 또는 하이픈을 사용하세요.", "Недопустимое имя команды или Agent. Используйте буквы, цифры, подчёркивания и дефисы.")
	addAux(KeyAuxSwarmMailboxFailed, "The team mailbox operation failed.", "团队邮箱操作失败。", "Der Team-Postfachvorgang ist fehlgeschlagen.", "チームのメールボックス操作に失敗しました。", "팀 메일함 작업에 실패했습니다.", "Операция с почтовым ящиком команды завершилась ошибкой.")
	addAux(KeyAuxSwarmFailed, "The team operation failed.", "团队操作失败。", "Der Teamvorgang ist fehlgeschlagen.", "チーム操作に失敗しました。", "팀 작업에 실패했습니다.", "Операция с командой завершилась ошибкой.")
	addAux(KeyAuxSwarmPaneTitle, "Could not set the tmux pane title; continuing", "无法设置 tmux pane 标题；将继续执行", "Der Titel des tmux-Panes konnte nicht gesetzt werden; Vorgang wird fortgesetzt", "tmux pane のタイトルを設定できませんでした。処理を続行します", "tmux pane 제목을 설정할 수 없습니다. 계속 진행합니다", "Не удалось задать заголовок панели tmux; работа продолжена")
	addAux(KeyAuxSwarmPaneBorder, "Could not set the tmux pane border color; continuing", "无法设置 tmux pane 边框颜色；将继续执行", "Die Rahmenfarbe des tmux-Panes konnte nicht gesetzt werden; Vorgang wird fortgesetzt", "tmux pane の枠色を設定できませんでした。処理を続行します", "tmux pane 테두리 색상을 설정할 수 없습니다. 계속 진행합니다", "Не удалось задать цвет рамки панели tmux; работа продолжена")
	addAux(KeyAuxSwarmLayout, "Could not select the tmux layout; continuing", "无法选择 tmux 布局；将继续执行", "Das tmux-Layout konnte nicht ausgewählt werden; Vorgang wird fortgesetzt", "tmux レイアウトを選択できませんでした。処理を続行します", "tmux 레이아웃을 선택할 수 없습니다. 계속 진행합니다", "Не удалось выбрать раскладку tmux; работа продолжена")

	addAux(KeyAuxSkillNotFound, "The requested skill was not found.", "未找到请求的 skill。", "Der angeforderte Skill wurde nicht gefunden.", "指定された skill が見つかりません。", "요청한 skill을 찾을 수 없습니다.", "Запрошенный skill не найден.")
	addAux(KeyAuxSkillRevisionConflict, "Skill settings changed elsewhere. Refresh and try again.", "Skill 设置已在其他位置发生更改。请刷新后重试。", "Die Skill-Einstellungen wurden anderweitig geändert. Aktualisiere und versuche es erneut.", "Skill 設定が別の場所で変更されました。更新してから再試行してください。", "Skill 설정이 다른 곳에서 변경되었습니다. 새로고침한 후 다시 시도하세요.", "Настройки skill были изменены в другом месте. Обновите данные и повторите попытку.")
	addAux(KeyAuxSkillManagedReadOnly, "This skill is controlled by managed policy and cannot be changed here.", "此 skill 由托管策略控制，无法在此处更改。", "Dieser Skill wird durch eine verwaltete Richtlinie gesteuert und kann hier nicht geändert werden.", "この skill は管理ポリシーによって制御されているため、ここでは変更できません。", "이 skill은 관리형 정책으로 제어되므로 여기에서 변경할 수 없습니다.", "Этот skill управляется политикой и не может быть изменён здесь.")
	addAux(KeyAuxSkillInvalidScope, "That skill setting cannot be changed at this scope.", "无法在此作用域更改该 skill 设置。", "Diese Skill-Einstellung kann in diesem Geltungsbereich nicht geändert werden.", "このスコープでは skill 設定を変更できません。", "이 범위에서는 해당 skill 설정을 변경할 수 없습니다.", "Эту настройку skill нельзя изменить в данной области.")
	addAux(KeyAuxSkillInvalidSession, "The skill operation requires a valid session.", "Skill 操作需要有效会话。", "Der Skill-Vorgang erfordert eine gültige Sitzung.", "Skill 操作には有効なセッションが必要です。", "Skill 작업에는 유효한 세션이 필요합니다.", "Для операции skill требуется действующий сеанс.")
	addAux(KeyAuxSkillFailed, "The skill operation failed.", "Skill 操作失败。", "Der Skill-Vorgang ist fehlgeschlagen.", "Skill 操作に失敗しました。", "Skill 작업에 실패했습니다.", "Операция skill завершилась ошибкой.")
	addAux(KeyAuxMCPPromptDescription, "MCP prompt: %s", "MCP prompt：%s", "MCP-Prompt: %s", "MCP prompt: %s", "MCP prompt: %s", "MCP prompt: %s")

	addAux(KeyAuxMCPToolError, "MCP error: %s", "MCP 工具错误：%s", "MCP-Tool-Fehler: %s", "MCP ツールエラー: %s", "MCP 도구 오류: %s", "Ошибка инструмента MCP: %s")
}

func addAux(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
	}
}
