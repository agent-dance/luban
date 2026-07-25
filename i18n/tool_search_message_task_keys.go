package i18n

const (
	KeyToolSearchGlobInvalidResult          Key = "tool.search.glob_invalid_result"
	KeyToolSearchPatternRequired            Key = "tool.search.pattern_required"
	KeyToolSearchNoFiles                    Key = "tool.search.no_files"
	KeyToolSearchResultsTruncated           Key = "tool.search.results_truncated"
	KeyToolSearchGrepInvalidResult          Key = "tool.search.grep_invalid_result"
	KeyToolSearchInvalidInput               Key = "tool.search.invalid_input"
	KeyToolSearchInvalidOutputMode          Key = "tool.search.invalid_output_mode"
	KeyToolSearchNoMatches                  Key = "tool.search.no_matches"
	KeyToolSearchShowingPagination          Key = "tool.search.showing_pagination"
	KeyToolSearchFoundTotalAcross           Key = "tool.search.found_total_across"
	KeyToolSearchOccurrence                 Key = "tool.search.occurrence"
	KeyToolSearchOccurrences                Key = "tool.search.occurrences"
	KeyToolSearchFile                       Key = "tool.search.file"
	KeyToolSearchFiles                      Key = "tool.search.files"
	KeyToolSearchWithPagination             Key = "tool.search.with_pagination"
	KeyToolSearchFoundFiles                 Key = "tool.search.found_files"
	KeyToolSearchLimit                      Key = "tool.search.limit"
	KeyToolSearchOffset                     Key = "tool.search.offset"
	KeyToolSearchUNCNotAllowed              Key = "tool.search.unc_not_allowed"
	KeyToolSearchPathNotDirectory           Key = "tool.search.path_not_directory"
	KeyToolSearchPathOutsideAllowed         Key = "tool.search.path_outside_allowed"
	KeyToolSearchPathResolvesOutsideAllowed Key = "tool.search.path_resolves_outside_allowed"
	KeyToolSearchPathMissingAtCWD           Key = "tool.search.path_missing_at_cwd"
	KeyToolSearchDirectoryMissingAtCWD      Key = "tool.search.directory_missing_at_cwd"
	KeyToolSearchDidYouMean                 Key = "tool.search.did_you_mean"
	KeyToolSearchRipgrepTimedOut            Key = "tool.search.ripgrep_timed_out"
	KeyToolSearchRipgrepRetryFailed         Key = "tool.search.ripgrep_retry_failed"
	KeyToolSearchRipgrepCriticalError       Key = "tool.search.ripgrep_critical_error"
	KeyToolSearchRipgrepFailed              Key = "tool.search.ripgrep_failed"

	KeyToolSendUserMessageInvalidInput               Key = "tool.send_user_message.invalid_input"
	KeyToolSendUserMessageInvalidStatus              Key = "tool.send_user_message.invalid_status"
	KeyToolSendUserMessageMessageRequired            Key = "tool.send_user_message.message_required"
	KeyToolSendUserMessageAttachmentsMustBeArray     Key = "tool.send_user_message.attachments_must_be_array"
	KeyToolSendUserMessageEncodeFailed               Key = "tool.send_user_message.encode_failed"
	KeyToolSendUserMessageAttachmentMissing          Key = "tool.send_user_message.attachment_missing"
	KeyToolSendUserMessageAttachmentPermissionDenied Key = "tool.send_user_message.attachment_permission_denied"
	KeyToolSendUserMessageInspectAttachment          Key = "tool.send_user_message.inspect_attachment"
	KeyToolSendUserMessageAttachmentNotRegular       Key = "tool.send_user_message.attachment_not_regular"
	KeyToolSendUserMessageOpenAttachment             Key = "tool.send_user_message.open_attachment"
	KeyToolSendUserMessageCloseAttachment            Key = "tool.send_user_message.close_attachment"
	KeyToolSendUserMessageEmptyWorkingDirectory      Key = "tool.send_user_message.empty_working_directory"
	KeyToolSendUserMessageEmptyAttachmentPath        Key = "tool.send_user_message.empty_attachment_path"
	KeyToolSendUserMessageExpandAttachment           Key = "tool.send_user_message.expand_attachment"
	KeyToolSendUserMessageResolveAttachment          Key = "tool.send_user_message.resolve_attachment"
	KeyToolSendUserMessageDelivered                  Key = "tool.send_user_message.delivered"
	KeyToolSendUserMessageOneAttachmentIncluded      Key = "tool.send_user_message.one_attachment_included"
	KeyToolSendUserMessageAttachmentsIncluded        Key = "tool.send_user_message.attachments_included"

	KeyToolSendMessageUDSSendFailed         Key = "tool.send_message.uds_send_failed"
	KeyToolSendMessageUDSSent               Key = "tool.send_message.uds_sent"
	KeyToolSendMessageNoBroadcastRecipients Key = "tool.send_message.no_broadcast_recipients"
	KeyToolSendMessageBroadcastSent         Key = "tool.send_message.broadcast_sent"
	KeyToolSendMessageInboxSent             Key = "tool.send_message.inbox_sent"
	KeyToolSendMessageShutdownRequestSent   Key = "tool.send_message.shutdown_request_sent"
	KeyToolSendMessageShutdownApproved      Key = "tool.send_message.shutdown_approved"
	KeyToolSendMessageShutdownRejected      Key = "tool.send_message.shutdown_rejected"

	KeyToolTaskInputFieldRequired    Key = "tool.task.input_field_required"
	KeyToolTaskInputFieldString      Key = "tool.task.input_field_string"
	KeyToolTaskInputFieldStringArray Key = "tool.task.input_field_string_array"
	KeyToolTaskInputMetadataObject   Key = "tool.task.input_metadata_object"
	KeyToolTaskCreated               Key = "tool.task.created"
	KeyToolTaskCreateFailed          Key = "tool.task.create_failed"
	KeyToolTaskCreatedHookFeedback   Key = "tool.task.created_hook_feedback"
	KeyToolTaskListInvalidResult     Key = "tool.task.list_invalid_result"
	KeyToolTaskNoTasks               Key = "tool.task.no_tasks"
	KeyToolTaskBlockedBy             Key = "tool.task.blocked_by"
	KeyToolTaskNotFound              Key = "tool.task.not_found"
	KeyToolTaskHeading               Key = "tool.task.heading"
	KeyToolTaskStatus                Key = "tool.task.status"
	KeyToolTaskDescription           Key = "tool.task.description"
	KeyToolTaskGetBlockedBy          Key = "tool.task.get_blocked_by"
	KeyToolTaskBlocks                Key = "tool.task.blocks"
	KeyToolTaskInvalidStatus         Key = "tool.task.invalid_status"
	KeyToolTaskIDRequired            Key = "tool.task.id_required"
	KeyToolTaskCompletedHookFeedback Key = "tool.task.completed_hook_feedback"
	KeyToolTaskBackgroundUnavailable Key = "tool.task.background_unavailable"
	KeyToolTaskBackgroundIDRequired  Key = "tool.task.background_id_required"
	KeyToolTaskStopped               Key = "tool.task.stopped"
	KeyToolTaskBackgroundNotFound    Key = "tool.task.background_not_found"
	KeyToolTaskReadOutputFailed      Key = "tool.task.read_output_failed"
	KeyToolTaskOutputTruncated       Key = "tool.task.output_truncated"
	KeyToolTaskUpdateFailed          Key = "tool.task.update_failed"
	KeyToolTaskUpdated               Key = "tool.task.updated"
	KeyToolTaskUpdatedFields         Key = "tool.task.updated_fields"
)

func init() {
	addToolSearchMessageTask(KeyToolSearchGlobInvalidResult, "Glob returned an invalid typed result", "Glob 返回了无效的类型化结果", "Glob gab ein ungültiges typisiertes Ergebnis zurück", "Glob が無効な型付き結果を返しました", "Glob이 잘못된 형식의 결과를 반환했습니다", "Glob вернул недопустимый типизированный результат")
	addToolSearchMessageTask(KeyToolSearchPatternRequired, "'pattern' parameter is required", "必须提供参数 'pattern'", "Der Parameter 'pattern' ist erforderlich", "パラメーター 'pattern' は必須です", "'pattern' 매개변수가 필요합니다", "Требуется параметр 'pattern'")
	addToolSearchMessageTask(KeyToolSearchNoFiles, "No files found", "未找到文件", "Keine Dateien gefunden", "ファイルが見つかりません", "파일을 찾을 수 없습니다", "Файлы не найдены")
	addToolSearchMessageTask(KeyToolSearchResultsTruncated, "(Results are truncated. Consider using a more specific path or pattern.)", "（结果已截断，请使用更具体的路径或模式。）", "(Ergebnisse wurden gekürzt. Verwende einen genaueren Pfad oder ein genaueres Muster.)", "（結果は省略されています。より具体的なパスまたはパターンを指定してください。）", "(결과가 잘렸습니다. 더 구체적인 경로나 패턴을 사용하세요.)", "(Результаты усечены. Укажите более точный путь или шаблон.)")
	addToolSearchMessageTask(KeyToolSearchGrepInvalidResult, "Grep returned an invalid typed result", "Grep 返回了无效的类型化结果", "Grep gab ein ungültiges typisiertes Ergebnis zurück", "Grep が無効な型付き結果を返しました", "Grep이 잘못된 형식의 결과를 반환했습니다", "Grep вернул недопустимый типизированный результат")
	addToolSearchMessageTask(KeyToolSearchInvalidInput, "invalid input: %s", "输入无效：%s", "Ungültige Eingabe: %s", "入力が無効です: %s", "잘못된 입력: %s", "Недопустимый ввод: %s")
	addToolSearchMessageTask(KeyToolSearchInvalidOutputMode, "invalid output_mode: %s", "output_mode 无效：%s", "Ungültiger output_mode: %s", "output_mode が無効です: %s", "잘못된 output_mode: %s", "Недопустимый output_mode: %s")
	addToolSearchMessageTask(KeyToolSearchNoMatches, "No matches found", "未找到匹配项", "Keine Treffer gefunden", "一致する結果がありません", "일치하는 항목이 없습니다", "Совпадения не найдены")
	addToolSearchMessageTask(KeyToolSearchShowingPagination, "[Showing results with pagination = %s]", "[显示分页结果：%s]", "[Ergebnisse mit Paginierung: %s]", "[ページ指定された結果を表示: %s]", "[페이지 지정 결과 표시: %s]", "[Показаны результаты с пагинацией: %s]")
	addToolSearchMessageTask(KeyToolSearchFoundTotalAcross, "Found %d total %s across %d %s.", "共在 %d 个%s中找到 %d 个%s。", "Insgesamt %d %s in %d %s gefunden.", "%d %sで合計%d件の%sが見つかりました。", "%d개 %s에서 총 %d개 %s을(를) 찾았습니다.", "Всего найдено %d %s в %d %s.")
	addToolSearchMessageTask(KeyToolSearchOccurrence, "occurrence", "匹配项", "Treffer", "一致", "일치 항목", "совпадение")
	addToolSearchMessageTask(KeyToolSearchOccurrences, "occurrences", "匹配项", "Treffer", "一致", "일치 항목", "совпадений")
	addToolSearchMessageTask(KeyToolSearchFile, "file", "文件", "Datei", "ファイル", "파일", "файле")
	addToolSearchMessageTask(KeyToolSearchFiles, "files", "文件", "Dateien", "ファイル", "파일", "файлах")
	addToolSearchMessageTask(KeyToolSearchWithPagination, " with pagination = %s", "；分页参数：%s", " mit Paginierung = %s", "（ページ指定: %s）", " (페이지 지정: %s)", " с пагинацией = %s")
	addToolSearchMessageTask(KeyToolSearchFoundFiles, "Found %d %s", "找到 %d 个%s", "%d %s gefunden", "%d %sが見つかりました", "%d개 %s을(를) 찾았습니다", "Найдено %d %s")
	addToolSearchMessageTask(KeyToolSearchLimit, "limit: %d", "上限：%d", "Limit: %d", "上限: %d", "제한: %d", "лимит: %d")
	addToolSearchMessageTask(KeyToolSearchOffset, "offset: %d", "偏移量：%d", "Offset: %d", "オフセット: %d", "오프셋: %d", "смещение: %d")
	addToolSearchMessageTask(KeyToolSearchUNCNotAllowed, "UNC paths are not allowed: %s", "不允许使用 UNC 路径：%s", "UNC-Pfade sind nicht zulässig: %s", "UNC パスは使用できません: %s", "UNC 경로는 허용되지 않습니다: %s", "UNC-пути не разрешены: %s")
	addToolSearchMessageTask(KeyToolSearchPathNotDirectory, "Path is not a directory: %s", "路径不是目录：%s", "Pfad ist kein Verzeichnis: %s", "パスはディレクトリではありません: %s", "경로가 디렉터리가 아닙니다: %s", "Путь не является каталогом: %s")
	addToolSearchMessageTask(KeyToolSearchPathOutsideAllowed, "path is outside allowed directories: %s", "路径位于允许的目录之外：%s", "Pfad liegt außerhalb der zulässigen Verzeichnisse: %s", "パスは許可されたディレクトリの外部です: %s", "경로가 허용된 디렉터리 밖에 있습니다: %s", "Путь находится вне разрешённых каталогов: %s")
	addToolSearchMessageTask(KeyToolSearchPathResolvesOutsideAllowed, "path resolves outside allowed directories: %s", "路径解析后位于允许的目录之外：%s", "Aufgelöster Pfad liegt außerhalb der zulässigen Verzeichnisse: %s", "解決後のパスは許可されたディレクトリの外部です: %s", "확인된 경로가 허용된 디렉터리 밖에 있습니다: %s", "Разрешённый путь находится вне разрешённых каталогов: %s")
	addToolSearchMessageTask(KeyToolSearchPathMissingAtCWD, "Path does not exist: %s. Current working directory is %s.", "路径不存在：%s。当前工作目录为 %s。", "Pfad existiert nicht: %s. Aktuelles Arbeitsverzeichnis ist %s.", "パスが存在しません: %s。現在の作業ディレクトリは %s です。", "경로가 존재하지 않습니다: %s. 현재 작업 디렉터리는 %s입니다.", "Путь не существует: %s. Текущий рабочий каталог: %s.")
	addToolSearchMessageTask(KeyToolSearchDirectoryMissingAtCWD, "Directory does not exist: %s. Current working directory is %s.", "目录不存在：%s。当前工作目录为 %s。", "Verzeichnis existiert nicht: %s. Aktuelles Arbeitsverzeichnis ist %s.", "ディレクトリが存在しません: %s。現在の作業ディレクトリは %s です。", "디렉터리가 존재하지 않습니다: %s. 현재 작업 디렉터리는 %s입니다.", "Каталог не существует: %s. Текущий рабочий каталог: %s.")
	addToolSearchMessageTask(KeyToolSearchDidYouMean, " Did you mean %s?", " 是否要使用 %s？", " Meintest du %s?", " %s のことですか？", " %s을(를) 의미하셨나요?", " Возможно, имелось в виду %s?")
	addToolSearchMessageTask(KeyToolSearchRipgrepTimedOut, "Ripgrep search timed out", "Ripgrep 搜索超时", "Zeitüberschreitung bei der Ripgrep-Suche", "Ripgrep 検索がタイムアウトしました", "Ripgrep 검색 시간이 초과되었습니다", "Истекло время ожидания поиска Ripgrep")
	addToolSearchMessageTask(KeyToolSearchRipgrepRetryFailed, "ripgrep -j 1 retry failed: %s", "ripgrep 使用 -j 1 重试失败：%s", "ripgrep-Wiederholung mit -j 1 fehlgeschlagen: %s", "ripgrep の -j 1 再試行に失敗しました: %s", "ripgrep -j 1 재시도 실패: %s", "Повторный запуск ripgrep с -j 1 завершился ошибкой: %s")
	addToolSearchMessageTask(KeyToolSearchRipgrepCriticalError, "ripgrep critical error: %s", "ripgrep 严重错误：%s", "Kritischer ripgrep-Fehler: %s", "ripgrep の重大なエラー: %s", "ripgrep 심각한 오류: %s", "Критическая ошибка ripgrep: %s")
	addToolSearchMessageTask(KeyToolSearchRipgrepFailed, "ripgrep failed: %s", "ripgrep 执行失败：%s", "ripgrep fehlgeschlagen: %s", "ripgrep に失敗しました: %s", "ripgrep 실패: %s", "Ошибка ripgrep: %s")

	addToolSearchMessageTask(KeyToolSendUserMessageInvalidInput, "invalid SendUserMessage input: %v", "SendUserMessage 输入无效：%v", "Ungültige SendUserMessage-Eingabe: %v", "SendUserMessage の入力が無効です: %v", "잘못된 SendUserMessage 입력: %v", "Недопустимый ввод SendUserMessage: %v")
	addToolSearchMessageTask(KeyToolSendUserMessageInvalidStatus, "invalid status %q: expected one of %q or %q", "状态 %q 无效；应为 %q 或 %q", "Ungültiger Status %q: erwartet wird %q oder %q", "ステータス %q は無効です。%q または %q を指定してください", "잘못된 상태 %q: %q 또는 %q이(가) 필요합니다", "Недопустимый статус %q: ожидался %q или %q")
	addToolSearchMessageTask(KeyToolSendUserMessageMessageRequired, "message is required", "必须提供 message", "message ist erforderlich", "message は必須です", "message가 필요합니다", "Требуется message")
	addToolSearchMessageTask(KeyToolSendUserMessageAttachmentsMustBeArray, "invalid SendUserMessage input: attachments must be an array of strings", "SendUserMessage 输入无效：attachments 必须是字符串数组", "Ungültige SendUserMessage-Eingabe: attachments muss ein String-Array sein", "SendUserMessage の入力が無効です: attachments は文字列配列である必要があります", "잘못된 SendUserMessage 입력: attachments는 문자열 배열이어야 합니다", "Недопустимый ввод SendUserMessage: attachments должен быть массивом строк")
	addToolSearchMessageTask(KeyToolSendUserMessageEncodeFailed, "encode SendUserMessage output", "编码 SendUserMessage 输出失败", "SendUserMessage-Ausgabe konnte nicht codiert werden", "SendUserMessage の出力をエンコードできませんでした", "SendUserMessage 출력을 인코딩하지 못했습니다", "Не удалось кодировать вывод SendUserMessage")
	addToolSearchMessageTask(KeyToolSendUserMessageAttachmentMissing, "attachment %q does not exist. Current working directory: %s", "附件 %q 不存在。当前工作目录：%s", "Anhang %q existiert nicht. Aktuelles Arbeitsverzeichnis: %s", "添付ファイル %q が存在しません。現在の作業ディレクトリ: %s", "첨부 파일 %q이(가) 없습니다. 현재 작업 디렉터리: %s", "Вложение %q не существует. Текущий рабочий каталог: %s")
	addToolSearchMessageTask(KeyToolSendUserMessageAttachmentPermissionDenied, "attachment %q is not accessible (permission denied)", "无法访问附件 %q（权限不足）", "Auf Anhang %q kann nicht zugegriffen werden (Zugriff verweigert)", "添付ファイル %q にアクセスできません（権限がありません）", "첨부 파일 %q에 접근할 수 없습니다(권한 거부)", "Нет доступа к вложению %q (доступ запрещён)")
	addToolSearchMessageTask(KeyToolSendUserMessageInspectAttachment, "inspect attachment %q: %v", "检查附件 %q 失败：%v", "Anhang %q konnte nicht geprüft werden: %v", "添付ファイル %q を確認できませんでした: %v", "첨부 파일 %q을(를) 검사하지 못했습니다: %v", "Не удалось проверить вложение %q: %v")
	addToolSearchMessageTask(KeyToolSendUserMessageAttachmentNotRegular, "attachment %q is not a regular file", "附件 %q 不是普通文件", "Anhang %q ist keine reguläre Datei", "添付ファイル %q は通常のファイルではありません", "첨부 파일 %q이(가) 일반 파일이 아닙니다", "Вложение %q не является обычным файлом")
	addToolSearchMessageTask(KeyToolSendUserMessageOpenAttachment, "open attachment %q: %v", "打开附件 %q 失败：%v", "Anhang %q konnte nicht geöffnet werden: %v", "添付ファイル %q を開けませんでした: %v", "첨부 파일 %q을(를) 열지 못했습니다: %v", "Не удалось открыть вложение %q: %v")
	addToolSearchMessageTask(KeyToolSendUserMessageCloseAttachment, "close attachment %q: %v", "关闭附件 %q 失败：%v", "Anhang %q konnte nicht geschlossen werden: %v", "添付ファイル %q を閉じられませんでした: %v", "첨부 파일 %q을(를) 닫지 못했습니다: %v", "Не удалось закрыть вложение %q: %v")
	addToolSearchMessageTask(KeyToolSendUserMessageEmptyWorkingDirectory, "resolve SendUserMessage working directory: empty path", "无法解析 SendUserMessage 工作目录：路径为空", "SendUserMessage-Arbeitsverzeichnis kann nicht aufgelöst werden: leerer Pfad", "SendUserMessage の作業ディレクトリを解決できません: パスが空です", "SendUserMessage 작업 디렉터리를 확인할 수 없습니다: 빈 경로", "Не удалось определить рабочий каталог SendUserMessage: пустой путь")
	addToolSearchMessageTask(KeyToolSendUserMessageEmptyAttachmentPath, "attachment path must not be empty", "附件路径不能为空", "Anhangspfad darf nicht leer sein", "添付ファイルのパスは空にできません", "첨부 파일 경로는 비워 둘 수 없습니다", "Путь к вложению не должен быть пустым")
	addToolSearchMessageTask(KeyToolSendUserMessageExpandAttachment, "expand attachment %q: %v", "展开附件路径 %q 失败：%v", "Anhangspfad %q konnte nicht erweitert werden: %v", "添付ファイルのパス %q を展開できませんでした: %v", "첨부 파일 경로 %q을(를) 확장하지 못했습니다: %v", "Не удалось развернуть путь вложения %q: %v")
	addToolSearchMessageTask(KeyToolSendUserMessageResolveAttachment, "resolve attachment %q: %v", "解析附件路径 %q 失败：%v", "Anhang %q konnte nicht aufgelöst werden: %v", "添付ファイル %q を解決できませんでした: %v", "첨부 파일 %q을(를) 확인하지 못했습니다: %v", "Не удалось разрешить путь вложения %q: %v")
	addToolSearchMessageTask(KeyToolSendUserMessageDelivered, "Message delivered to user.", "消息已发送给用户。", "Nachricht wurde an den Benutzer gesendet.", "ユーザーにメッセージを送信しました。", "사용자에게 메시지를 보냈습니다.", "Сообщение отправлено пользователю.")
	addToolSearchMessageTask(KeyToolSendUserMessageOneAttachmentIncluded, " (1 attachment included)", "（已附加 1 个附件）", " (1 Anhang enthalten)", "（添付ファイル1件を含む）", " (첨부 파일 1개 포함)", " (включено 1 вложение)")
	addToolSearchMessageTask(KeyToolSendUserMessageAttachmentsIncluded, " (%d attachments included)", "（已附加 %d 个附件）", " (%d Anhänge enthalten)", "（添付ファイル%d件を含む）", " (첨부 파일 %d개 포함)", " (включено вложений: %d)")

	addToolSearchMessageTask(KeyToolSendMessageUDSSendFailed, "Failed to send to uds:%s: %s", "发送到 uds:%s 失败：%s", "Senden an uds:%s fehlgeschlagen: %s", "uds:%s への送信に失敗しました: %s", "uds:%s에 보내지 못했습니다: %s", "Не удалось отправить в uds:%s: %s")
	addToolSearchMessageTask(KeyToolSendMessageUDSSent, "“%s” -> uds:%s", "“%s” → uds:%s", "„%s“ -> uds:%s", "「%s」→ uds:%s", "“%s” → uds:%s", "«%s» -> uds:%s")
	addToolSearchMessageTask(KeyToolSendMessageNoBroadcastRecipients, "No teammates to broadcast to (you are the only team member)", "没有可广播的队友（你是团队中唯一的成员）", "Keine Teammitglieder für die Übertragung vorhanden (du bist das einzige Teammitglied)", "ブロードキャスト先のチームメイトがいません（チームメンバーは自分だけです）", "브로드캐스트할 팀원이 없습니다(유일한 팀원입니다)", "Нет участников для рассылки (вы единственный участник команды)")
	addToolSearchMessageTask(KeyToolSendMessageBroadcastSent, "Message broadcast to %d teammate(s): %s", "消息已广播给 %d 名队友：%s", "Nachricht an %d Teammitglied(er) gesendet: %s", "%d人のチームメイトにメッセージをブロードキャストしました: %s", "팀원 %d명에게 메시지를 브로드캐스트했습니다: %s", "Сообщение разослано участникам (%d): %s")
	addToolSearchMessageTask(KeyToolSendMessageInboxSent, "Message sent to %s's inbox", "消息已发送到 %s 的收件箱", "Nachricht an den Posteingang von %s gesendet", "%s の受信トレイにメッセージを送信しました", "%s의 받은 편지함으로 메시지를 보냈습니다", "Сообщение отправлено во входящие %s")
	addToolSearchMessageTask(KeyToolSendMessageShutdownRequestSent, "Shutdown request sent to %s. Request ID: %s", "已向 %s 发送关闭请求。请求 ID：%s", "Beendigungsanfrage an %s gesendet. Anfrage-ID: %s", "%s に終了リクエストを送信しました。リクエスト ID: %s", "%s에게 종료 요청을 보냈습니다. 요청 ID: %s", "Запрос на завершение отправлен %s. ID запроса: %s")
	addToolSearchMessageTask(KeyToolSendMessageShutdownApproved, "Shutdown approved. Sent confirmation to %s. Agent %s is now exiting.", "关闭请求已批准。已向 %s 发送确认，Agent %s 正在退出。", "Beendigung genehmigt. Bestätigung an %s gesendet. Agent %s wird jetzt beendet.", "終了を承認しました。%s に確認を送信し、Agent %s は終了します。", "종료를 승인했습니다. %s에게 확인을 보냈으며 Agent %s이(가) 종료됩니다.", "Завершение одобрено. Подтверждение отправлено %s. Agent %s завершает работу.")
	addToolSearchMessageTask(KeyToolSendMessageShutdownRejected, "Shutdown rejected. Reason: %q. Continuing to work.", "已拒绝关闭请求。原因：%q。将继续工作。", "Beendigung abgelehnt. Grund: %q. Arbeit wird fortgesetzt.", "終了を拒否しました。理由: %q。作業を続行します。", "종료를 거부했습니다. 이유: %q. 작업을 계속합니다.", "Завершение отклонено. Причина: %q. Работа продолжается.")

	addToolSearchMessageTask(KeyToolTaskInputFieldRequired, "Error: invalid input: %s is required", "错误：输入无效：必须提供 %s", "Fehler: Ungültige Eingabe: %s ist erforderlich", "エラー: 入力が無効です: %s は必須です", "오류: 잘못된 입력: %s이(가) 필요합니다", "Ошибка: недопустимый ввод: требуется %s")
	addToolSearchMessageTask(KeyToolTaskInputFieldString, "Error: invalid input: %s must be a string", "错误：输入无效：%s 必须是字符串", "Fehler: Ungültige Eingabe: %s muss ein String sein", "エラー: 入力が無効です: %s は文字列である必要があります", "오류: 잘못된 입력: %s은(는) 문자열이어야 합니다", "Ошибка: недопустимый ввод: %s должен быть строкой")
	addToolSearchMessageTask(KeyToolTaskInputFieldStringArray, "Error: invalid input: %s must be an array of strings", "错误：输入无效：%s 必须是字符串数组", "Fehler: Ungültige Eingabe: %s muss ein String-Array sein", "エラー: 入力が無効です: %s は文字列配列である必要があります", "오류: 잘못된 입력: %s은(는) 문자열 배열이어야 합니다", "Ошибка: недопустимый ввод: %s должен быть массивом строк")
	addToolSearchMessageTask(KeyToolTaskInputMetadataObject, "Error: invalid input: metadata must be an object", "错误：输入无效：metadata 必须是对象", "Fehler: Ungültige Eingabe: metadata muss ein Objekt sein", "エラー: 入力が無効です: metadata はオブジェクトである必要があります", "오류: 잘못된 입력: metadata는 객체여야 합니다", "Ошибка: недопустимый ввод: metadata должен быть объектом")
	addToolSearchMessageTask(KeyToolTaskCreated, "Task #%s created successfully: %s", "任务 #%s 创建成功：%s", "Aufgabe #%s erfolgreich erstellt: %s", "タスク #%s を作成しました: %s", "작업 #%s을(를) 만들었습니다: %s", "Задача #%s успешно создана: %s")
	addToolSearchMessageTask(KeyToolTaskCreateFailed, "failed to create task: %v", "创建任务失败：%v", "Aufgabe konnte nicht erstellt werden: %v", "タスクを作成できませんでした: %v", "작업을 만들지 못했습니다: %v", "Не удалось создать задачу: %v")
	addToolSearchMessageTask(KeyToolTaskCreatedHookFeedback, "TaskCreated hook feedback:\n%s", "TaskCreated hook 反馈：\n%s", "Rückmeldung des TaskCreated-Hooks:\n%s", "TaskCreated hook のフィードバック:\n%s", "TaskCreated hook 피드백:\n%s", "Комментарий hook TaskCreated:\n%s")
	addToolSearchMessageTask(KeyToolTaskListInvalidResult, "TaskList returned an invalid typed result", "TaskList 返回了无效的类型化结果", "TaskList gab ein ungültiges typisiertes Ergebnis zurück", "TaskList が無効な型付き結果を返しました", "TaskList가 잘못된 형식의 결과를 반환했습니다", "TaskList вернул недопустимый типизированный результат")
	addToolSearchMessageTask(KeyToolTaskNoTasks, "No tasks found", "未找到任务", "Keine Aufgaben gefunden", "タスクが見つかりません", "작업을 찾을 수 없습니다", "Задачи не найдены")
	addToolSearchMessageTask(KeyToolTaskBlockedBy, " [blocked by %s]", " [被 %s 阻塞]", " [blockiert durch %s]", " [%s によりブロック中]", " [%s에 의해 차단됨]", " [заблокировано %s]")
	addToolSearchMessageTask(KeyToolTaskNotFound, "Task not found", "未找到任务", "Aufgabe nicht gefunden", "タスクが見つかりません", "작업을 찾을 수 없습니다", "Задача не найдена")
	addToolSearchMessageTask(KeyToolTaskHeading, "Task #%s: %s", "任务 #%s：%s", "Aufgabe #%s: %s", "タスク #%s: %s", "작업 #%s: %s", "Задача #%s: %s")
	addToolSearchMessageTask(KeyToolTaskStatus, "Status: %s", "状态：%s", "Status: %s", "ステータス: %s", "상태: %s", "Статус: %s")
	addToolSearchMessageTask(KeyToolTaskDescription, "Description: %s", "说明：%s", "Beschreibung: %s", "説明: %s", "설명: %s", "Описание: %s")
	addToolSearchMessageTask(KeyToolTaskGetBlockedBy, "Blocked by: %s", "被以下任务阻塞：%s", "Blockiert durch: %s", "ブロック元: %s", "차단한 작업: %s", "Заблокировано задачами: %s")
	addToolSearchMessageTask(KeyToolTaskBlocks, "Blocks: %s", "阻塞以下任务：%s", "Blockiert: %s", "ブロック対象: %s", "차단하는 작업: %s", "Блокирует: %s")
	addToolSearchMessageTask(KeyToolTaskInvalidStatus, "invalid status %q; must be pending, in_progress, completed, or deleted", "状态 %q 无效；必须为 pending、in_progress、completed 或 deleted", "Ungültiger Status %q; zulässig sind pending, in_progress, completed oder deleted", "ステータス %q は無効です。pending、in_progress、completed、deleted のいずれかを指定してください", "잘못된 상태 %q: pending, in_progress, completed 또는 deleted여야 합니다", "Недопустимый статус %q; допустимы pending, in_progress, completed или deleted")
	addToolSearchMessageTask(KeyToolTaskIDRequired, "taskId is required", "必须提供 taskId", "taskId ist erforderlich", "taskId は必須です", "taskId가 필요합니다", "Требуется taskId")
	addToolSearchMessageTask(KeyToolTaskCompletedHookFeedback, "TaskCompleted hook feedback:\n%s", "TaskCompleted hook 反馈：\n%s", "Rückmeldung des TaskCompleted-Hooks:\n%s", "TaskCompleted hook のフィードバック:\n%s", "TaskCompleted hook 피드백:\n%s", "Комментарий hook TaskCompleted:\n%s")
	addToolSearchMessageTask(KeyToolTaskBackgroundUnavailable, "background tasks are not available", "后台任务不可用", "Hintergrundaufgaben sind nicht verfügbar", "バックグラウンドタスクは利用できません", "백그라운드 작업을 사용할 수 없습니다", "Фоновые задачи недоступны")
	addToolSearchMessageTask(KeyToolTaskBackgroundIDRequired, "Missing required parameter: task_id", "缺少必填参数：task_id", "Erforderlicher Parameter fehlt: task_id", "必須パラメーターがありません: task_id", "필수 매개변수가 없습니다: task_id", "Отсутствует обязательный параметр: task_id")
	addToolSearchMessageTask(KeyToolTaskStopped, "Successfully stopped task: %s (%s)", "已成功停止任务：%s（%s）", "Aufgabe erfolgreich beendet: %s (%s)", "タスクを停止しました: %s（%s）", "작업을 중지했습니다: %s (%s)", "Задача успешно остановлена: %s (%s)")
	addToolSearchMessageTask(KeyToolTaskBackgroundNotFound, "No task found with ID: %s", "未找到 ID 为 %s 的任务", "Keine Aufgabe mit der ID %s gefunden", "ID %s のタスクが見つかりません", "ID가 %s인 작업을 찾을 수 없습니다", "Задача с ID %s не найдена")
	addToolSearchMessageTask(KeyToolTaskReadOutputFailed, "failed to read task output: %v", "读取任务输出失败：%v", "Aufgabenausgabe konnte nicht gelesen werden: %v", "タスク出力を読み取れませんでした: %v", "작업 출력을 읽지 못했습니다: %v", "Не удалось прочитать вывод задачи: %v")
	addToolSearchMessageTask(KeyToolTaskOutputTruncated, "[Truncated. Full output: %s]\n\n", "[输出已截断。完整输出：%s]\n\n", "[Gekürzt. Vollständige Ausgabe: %s]\n\n", "[出力は省略されました。完全な出力: %s]\n\n", "[출력이 잘렸습니다. 전체 출력: %s]\n\n", "[Вывод усечён. Полный вывод: %s]\n\n")
	addToolSearchMessageTask(KeyToolTaskUpdateFailed, "Task update failed", "任务更新失败", "Aufgabe konnte nicht aktualisiert werden", "タスクの更新に失敗しました", "작업 업데이트 실패", "Не удалось обновить задачу")
	addToolSearchMessageTask(KeyToolTaskUpdated, "Updated task #%s%s", "已更新任务 #%s%s", "Aufgabe #%s aktualisiert%s", "タスク #%s を更新しました%s", "작업 #%s 업데이트됨%s", "Задача #%s обновлена%s")
	addToolSearchMessageTask(KeyToolTaskUpdatedFields, "Updated task #%s %s%s", "已更新任务 #%s：%s%s", "Aufgabe #%s aktualisiert: %s%s", "タスク #%s を更新しました: %s%s", "작업 #%s 업데이트됨: %s%s", "Задача #%s обновлена: %s%s")
}

func addToolSearchMessageTask(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
