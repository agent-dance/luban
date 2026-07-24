package i18n

const (
	KeyToolLegacyCGlobInvalidResult          Key = "tool.legacy_c.search.glob_invalid_result"
	KeyToolLegacyCPatternRequired            Key = "tool.legacy_c.search.pattern_required"
	KeyToolLegacyCNoFiles                    Key = "tool.legacy_c.search.no_files"
	KeyToolLegacyCResultsTruncated           Key = "tool.legacy_c.search.results_truncated"
	KeyToolLegacyCGrepInvalidResult          Key = "tool.legacy_c.search.grep_invalid_result"
	KeyToolLegacyCInvalidInput               Key = "tool.legacy_c.search.invalid_input"
	KeyToolLegacyCInvalidOutputMode          Key = "tool.legacy_c.search.invalid_output_mode"
	KeyToolLegacyCNoMatches                  Key = "tool.legacy_c.search.no_matches"
	KeyToolLegacyCShowingPagination          Key = "tool.legacy_c.search.showing_pagination"
	KeyToolLegacyCFoundTotalAcross           Key = "tool.legacy_c.search.found_total_across"
	KeyToolLegacyCOccurrence                 Key = "tool.legacy_c.search.occurrence"
	KeyToolLegacyCOccurrences                Key = "tool.legacy_c.search.occurrences"
	KeyToolLegacyCFile                       Key = "tool.legacy_c.search.file"
	KeyToolLegacyCFiles                      Key = "tool.legacy_c.search.files"
	KeyToolLegacyCWithPagination             Key = "tool.legacy_c.search.with_pagination"
	KeyToolLegacyCFoundFiles                 Key = "tool.legacy_c.search.found_files"
	KeyToolLegacyCLimit                      Key = "tool.legacy_c.search.limit"
	KeyToolLegacyCOffset                     Key = "tool.legacy_c.search.offset"
	KeyToolLegacyCUNCNotAllowed              Key = "tool.legacy_c.search.unc_not_allowed"
	KeyToolLegacyCPathNotDirectory           Key = "tool.legacy_c.search.path_not_directory"
	KeyToolLegacyCPathOutsideAllowed         Key = "tool.legacy_c.search.path_outside_allowed"
	KeyToolLegacyCPathResolvesOutsideAllowed Key = "tool.legacy_c.search.path_resolves_outside_allowed"
	KeyToolLegacyCPathMissing                Key = "tool.legacy_c.search.path_missing"
	KeyToolLegacyCPathMissingAtCWD           Key = "tool.legacy_c.search.path_missing_at_cwd"
	KeyToolLegacyCDirectoryMissing           Key = "tool.legacy_c.search.directory_missing"
	KeyToolLegacyCDirectoryMissingAtCWD      Key = "tool.legacy_c.search.directory_missing_at_cwd"
	KeyToolLegacyCDidYouMean                 Key = "tool.legacy_c.search.did_you_mean"
	KeyToolLegacyCRipgrepTimedOut            Key = "tool.legacy_c.search.ripgrep_timed_out"
	KeyToolLegacyCRipgrepRetryFailed         Key = "tool.legacy_c.search.ripgrep_retry_failed"
	KeyToolLegacyCRipgrepCriticalError       Key = "tool.legacy_c.search.ripgrep_critical_error"
	KeyToolLegacyCRipgrepFailed              Key = "tool.legacy_c.search.ripgrep_failed"

	KeyToolLegacyCEnvNotSet             Key = "tool.legacy_c.shell.env_not_set"
	KeyToolLegacyCSetEnvFailed          Key = "tool.legacy_c.shell.set_env_failed"
	KeyToolLegacyCGetCWDFailed          Key = "tool.legacy_c.shell.get_cwd_failed"
	KeyToolLegacyCGetWDFailed           Key = "tool.legacy_c.shell.get_wd_failed"
	KeyToolLegacyCResolvePathFailed     Key = "tool.legacy_c.shell.resolve_path_failed"
	KeyToolLegacyCAccessDirectoryFailed Key = "tool.legacy_c.shell.access_directory_failed"
	KeyToolLegacyCPathIsNotDirectory    Key = "tool.legacy_c.shell.path_not_directory"
	KeyToolLegacyCChangeDirectoryFailed Key = "tool.legacy_c.shell.change_directory_failed"

	KeyToolLegacyCUptimeFailed      Key = "tool.legacy_c.system.uptime_failed"
	KeyToolLegacyCLastBootUptime    Key = "tool.legacy_c.system.last_boot_uptime"
	KeyToolLegacyCProcessListFailed Key = "tool.legacy_c.system.process_list_failed"
	KeyToolLegacyCInvalidPID        Key = "tool.legacy_c.system.invalid_pid"
	KeyToolLegacyCProcessKillFailed Key = "tool.legacy_c.system.process_kill_failed"
	KeyToolLegacyCCommandFailed     Key = "tool.legacy_c.system.command_failed"

	KeyToolLegacyCInvalidSendUserMessage      Key = "tool.legacy_c.send_user.invalid_input"
	KeyToolLegacyCInvalidStatus               Key = "tool.legacy_c.send_user.invalid_status"
	KeyToolLegacyCMessageRequired             Key = "tool.legacy_c.send_user.message_required"
	KeyToolLegacyCAttachmentsMustBeArray      Key = "tool.legacy_c.send_user.attachments_must_be_array"
	KeyToolLegacyCEncodeSendUserMessageFailed Key = "tool.legacy_c.send_user.encode_failed"
	KeyToolLegacyCAttachmentMissing           Key = "tool.legacy_c.send_user.attachment_missing"
	KeyToolLegacyCAttachmentPermissionDenied  Key = "tool.legacy_c.send_user.attachment_permission_denied"
	KeyToolLegacyCInspectAttachment           Key = "tool.legacy_c.send_user.inspect_attachment"
	KeyToolLegacyCAttachmentNotRegular        Key = "tool.legacy_c.send_user.attachment_not_regular"
	KeyToolLegacyCOpenAttachment              Key = "tool.legacy_c.send_user.open_attachment"
	KeyToolLegacyCCloseAttachment             Key = "tool.legacy_c.send_user.close_attachment"
	KeyToolLegacyCEmptyWorkingDirectory       Key = "tool.legacy_c.send_user.empty_working_directory"
	KeyToolLegacyCEmptyAttachmentPath         Key = "tool.legacy_c.send_user.empty_attachment_path"
	KeyToolLegacyCExpandAttachment            Key = "tool.legacy_c.send_user.expand_attachment"
	KeyToolLegacyCResolveAttachment           Key = "tool.legacy_c.send_user.resolve_attachment"
	KeyToolLegacyCMessageDelivered            Key = "tool.legacy_c.send_user.delivered"
	KeyToolLegacyCOneAttachmentIncluded       Key = "tool.legacy_c.send_user.one_attachment_included"
	KeyToolLegacyCAttachmentsIncluded         Key = "tool.legacy_c.send_user.attachments_included"

	KeyToolLegacyCAddressTargetEmpty        Key = "tool.legacy_c.send_message.address_target_empty"
	KeyToolLegacyCUDSSendFailed             Key = "tool.legacy_c.send_message.uds_send_failed"
	KeyToolLegacyCUDSSent                   Key = "tool.legacy_c.send_message.uds_sent"
	KeyToolLegacyCNoBroadcastRecipients     Key = "tool.legacy_c.send_message.no_broadcast_recipients"
	KeyToolLegacyCBroadcastSent             Key = "tool.legacy_c.send_message.broadcast_sent"
	KeyToolLegacyCInboxSent                 Key = "tool.legacy_c.send_message.inbox_sent"
	KeyToolLegacyCShutdownRequestSent       Key = "tool.legacy_c.send_message.shutdown_request_sent"
	KeyToolLegacyCShutdownResponseTarget    Key = "tool.legacy_c.send_message.shutdown_response_target"
	KeyToolLegacyCShutdownRequestIDRequired Key = "tool.legacy_c.send_message.shutdown_request_id_required"
	KeyToolLegacyCShutdownApproveRequired   Key = "tool.legacy_c.send_message.shutdown_approve_required"
	KeyToolLegacyCShutdownApproved          Key = "tool.legacy_c.send_message.shutdown_approved"
	KeyToolLegacyCShutdownRejectReason      Key = "tool.legacy_c.send_message.shutdown_reject_reason"
	KeyToolLegacyCShutdownRejected          Key = "tool.legacy_c.send_message.shutdown_rejected"
	KeyToolLegacyCPlanRequestIDRequired     Key = "tool.legacy_c.send_message.plan_request_id_required"
	KeyToolLegacyCPlanApproveRequired       Key = "tool.legacy_c.send_message.plan_approve_required"
	KeyToolLegacyCPlanLeadOnly              Key = "tool.legacy_c.send_message.plan_lead_only"
	KeyToolLegacyCPlanNeedsRevision         Key = "tool.legacy_c.send_message.plan_needs_revision"
	KeyToolLegacyCPlanApproved              Key = "tool.legacy_c.send_message.plan_approved"
	KeyToolLegacyCPlanRejected              Key = "tool.legacy_c.send_message.plan_rejected"
	KeyToolLegacyCUnsupportedStructuredType Key = "tool.legacy_c.send_message.unsupported_structured_type"
	KeyToolLegacyCMarshalResponseFailed     Key = "tool.legacy_c.send_message.marshal_response_failed"
	KeyToolLegacyCEncodeResultFailed        Key = "tool.legacy_c.send_message.encode_result_failed"

	KeyToolLegacyCInputFieldRequired        Key = "tool.legacy_c.task.input_field_required"
	KeyToolLegacyCInputFieldString          Key = "tool.legacy_c.task.input_field_string"
	KeyToolLegacyCInputFieldStringArray     Key = "tool.legacy_c.task.input_field_string_array"
	KeyToolLegacyCInputMetadataObject       Key = "tool.legacy_c.task.input_metadata_object"
	KeyToolLegacyCTaskCreated               Key = "tool.legacy_c.task.created"
	KeyToolLegacyCTaskCreateFailed          Key = "tool.legacy_c.task.create_failed"
	KeyToolLegacyCTaskCreatedHookFeedback   Key = "tool.legacy_c.task.created_hook_feedback"
	KeyToolLegacyCTaskListInvalidResult     Key = "tool.legacy_c.task.list_invalid_result"
	KeyToolLegacyCNoTasks                   Key = "tool.legacy_c.task.no_tasks"
	KeyToolLegacyCTaskBlockedBy             Key = "tool.legacy_c.task.blocked_by"
	KeyToolLegacyCTaskNotFound              Key = "tool.legacy_c.task.not_found"
	KeyToolLegacyCTaskHeading               Key = "tool.legacy_c.task.heading"
	KeyToolLegacyCTaskStatus                Key = "tool.legacy_c.task.status"
	KeyToolLegacyCTaskDescription           Key = "tool.legacy_c.task.description"
	KeyToolLegacyCTaskGetBlockedBy          Key = "tool.legacy_c.task.get_blocked_by"
	KeyToolLegacyCTaskBlocks                Key = "tool.legacy_c.task.blocks"
	KeyToolLegacyCInvalidTaskStatus         Key = "tool.legacy_c.task.invalid_status"
	KeyToolLegacyCTaskIDRequired            Key = "tool.legacy_c.task.id_required"
	KeyToolLegacyCTaskCompletedHookFeedback Key = "tool.legacy_c.task.completed_hook_feedback"
	KeyToolLegacyCBackgroundUnavailable     Key = "tool.legacy_c.task.background_unavailable"
	KeyToolLegacyCBackgroundIDRequired      Key = "tool.legacy_c.task.background_id_required"
	KeyToolLegacyCTaskStopped               Key = "tool.legacy_c.task.stopped"
	KeyToolLegacyCBackgroundTaskNotFound    Key = "tool.legacy_c.task.background_not_found"
	KeyToolLegacyCReadTaskOutputFailed      Key = "tool.legacy_c.task.read_output_failed"
	KeyToolLegacyCTaskOutputTruncated       Key = "tool.legacy_c.task.output_truncated"
	KeyToolLegacyCTaskCreationEmpty         Key = "tool.legacy_c.task.creation_empty"
	KeyToolLegacyCTaskUpdateFailed          Key = "tool.legacy_c.task.update_failed"
	KeyToolLegacyCTaskUpdated               Key = "tool.legacy_c.task.updated"
	KeyToolLegacyCTaskUpdatedFields         Key = "tool.legacy_c.task.updated_fields"
)

func init() {
	addToolLegacyC(KeyToolLegacyCGlobInvalidResult, "Glob returned an invalid typed result", "Glob 返回了无效的类型化结果", "Glob gab ein ungültiges typisiertes Ergebnis zurück", "Glob が無効な型付き結果を返しました", "Glob이 잘못된 형식의 결과를 반환했습니다", "Glob вернул недопустимый типизированный результат")
	addToolLegacyC(KeyToolLegacyCPatternRequired, "'pattern' parameter is required", "必须提供参数 'pattern'", "Der Parameter 'pattern' ist erforderlich", "パラメーター 'pattern' は必須です", "'pattern' 매개변수가 필요합니다", "Требуется параметр 'pattern'")
	addToolLegacyC(KeyToolLegacyCNoFiles, "No files found", "未找到文件", "Keine Dateien gefunden", "ファイルが見つかりません", "파일을 찾을 수 없습니다", "Файлы не найдены")
	addToolLegacyC(KeyToolLegacyCResultsTruncated, "(Results are truncated. Consider using a more specific path or pattern.)", "（结果已截断，请使用更具体的路径或模式。）", "(Ergebnisse wurden gekürzt. Verwende einen genaueren Pfad oder ein genaueres Muster.)", "（結果は省略されています。より具体的なパスまたはパターンを指定してください。）", "(결과가 잘렸습니다. 더 구체적인 경로나 패턴을 사용하세요.)", "(Результаты усечены. Укажите более точный путь или шаблон.)")
	addToolLegacyC(KeyToolLegacyCGrepInvalidResult, "Grep returned an invalid typed result", "Grep 返回了无效的类型化结果", "Grep gab ein ungültiges typisiertes Ergebnis zurück", "Grep が無効な型付き結果を返しました", "Grep이 잘못된 형식의 결과를 반환했습니다", "Grep вернул недопустимый типизированный результат")
	addToolLegacyC(KeyToolLegacyCInvalidInput, "invalid input: %s", "输入无效：%s", "Ungültige Eingabe: %s", "入力が無効です: %s", "잘못된 입력: %s", "Недопустимый ввод: %s")
	addToolLegacyC(KeyToolLegacyCInvalidOutputMode, "invalid output_mode: %s", "output_mode 无效：%s", "Ungültiger output_mode: %s", "output_mode が無効です: %s", "잘못된 output_mode: %s", "Недопустимый output_mode: %s")
	addToolLegacyC(KeyToolLegacyCNoMatches, "No matches found", "未找到匹配项", "Keine Treffer gefunden", "一致する結果がありません", "일치하는 항목이 없습니다", "Совпадения не найдены")
	addToolLegacyC(KeyToolLegacyCShowingPagination, "[Showing results with pagination = %s]", "[显示分页结果：%s]", "[Ergebnisse mit Paginierung: %s]", "[ページ指定された結果を表示: %s]", "[페이지 지정 결과 표시: %s]", "[Показаны результаты с пагинацией: %s]")
	addToolLegacyC(KeyToolLegacyCFoundTotalAcross, "Found %d total %s across %d %s.", "共在 %d 个%s中找到 %d 个%s。", "Insgesamt %d %s in %d %s gefunden.", "%d %sで合計%d件の%sが見つかりました。", "%d개 %s에서 총 %d개 %s을(를) 찾았습니다.", "Всего найдено %d %s в %d %s.")
	addToolLegacyC(KeyToolLegacyCOccurrence, "occurrence", "匹配项", "Treffer", "一致", "일치 항목", "совпадение")
	addToolLegacyC(KeyToolLegacyCOccurrences, "occurrences", "匹配项", "Treffer", "一致", "일치 항목", "совпадений")
	addToolLegacyC(KeyToolLegacyCFile, "file", "文件", "Datei", "ファイル", "파일", "файле")
	addToolLegacyC(KeyToolLegacyCFiles, "files", "文件", "Dateien", "ファイル", "파일", "файлах")
	addToolLegacyC(KeyToolLegacyCWithPagination, " with pagination = %s", "；分页参数：%s", " mit Paginierung = %s", "（ページ指定: %s）", " (페이지 지정: %s)", " с пагинацией = %s")
	addToolLegacyC(KeyToolLegacyCFoundFiles, "Found %d %s", "找到 %d 个%s", "%d %s gefunden", "%d %sが見つかりました", "%d개 %s을(를) 찾았습니다", "Найдено %d %s")
	addToolLegacyC(KeyToolLegacyCLimit, "limit: %d", "上限：%d", "Limit: %d", "上限: %d", "제한: %d", "лимит: %d")
	addToolLegacyC(KeyToolLegacyCOffset, "offset: %d", "偏移量：%d", "Offset: %d", "オフセット: %d", "오프셋: %d", "смещение: %d")
	addToolLegacyC(KeyToolLegacyCUNCNotAllowed, "UNC paths are not allowed: %s", "不允许使用 UNC 路径：%s", "UNC-Pfade sind nicht zulässig: %s", "UNC パスは使用できません: %s", "UNC 경로는 허용되지 않습니다: %s", "UNC-пути не разрешены: %s")
	addToolLegacyC(KeyToolLegacyCPathNotDirectory, "Path is not a directory: %s", "路径不是目录：%s", "Pfad ist kein Verzeichnis: %s", "パスはディレクトリではありません: %s", "경로가 디렉터리가 아닙니다: %s", "Путь не является каталогом: %s")
	addToolLegacyC(KeyToolLegacyCPathOutsideAllowed, "path is outside allowed directories: %s", "路径位于允许的目录之外：%s", "Pfad liegt außerhalb der zulässigen Verzeichnisse: %s", "パスは許可されたディレクトリの外部です: %s", "경로가 허용된 디렉터리 밖에 있습니다: %s", "Путь находится вне разрешённых каталогов: %s")
	addToolLegacyC(KeyToolLegacyCPathResolvesOutsideAllowed, "path resolves outside allowed directories: %s", "路径解析后位于允许的目录之外：%s", "Aufgelöster Pfad liegt außerhalb der zulässigen Verzeichnisse: %s", "解決後のパスは許可されたディレクトリの外部です: %s", "확인된 경로가 허용된 디렉터리 밖에 있습니다: %s", "Разрешённый путь находится вне разрешённых каталогов: %s")
	addToolLegacyC(KeyToolLegacyCPathMissing, "Path does not exist: %s", "路径不存在：%s", "Pfad existiert nicht: %s", "パスが存在しません: %s", "경로가 존재하지 않습니다: %s", "Путь не существует: %s")
	addToolLegacyC(KeyToolLegacyCPathMissingAtCWD, "Path does not exist: %s. Current working directory is %s.", "路径不存在：%s。当前工作目录为 %s。", "Pfad existiert nicht: %s. Aktuelles Arbeitsverzeichnis ist %s.", "パスが存在しません: %s。現在の作業ディレクトリは %s です。", "경로가 존재하지 않습니다: %s. 현재 작업 디렉터리는 %s입니다.", "Путь не существует: %s. Текущий рабочий каталог: %s.")
	addToolLegacyC(KeyToolLegacyCDirectoryMissing, "Directory does not exist: %s", "目录不存在：%s", "Verzeichnis existiert nicht: %s", "ディレクトリが存在しません: %s", "디렉터리가 존재하지 않습니다: %s", "Каталог не существует: %s")
	addToolLegacyC(KeyToolLegacyCDirectoryMissingAtCWD, "Directory does not exist: %s. Current working directory is %s.", "目录不存在：%s。当前工作目录为 %s。", "Verzeichnis existiert nicht: %s. Aktuelles Arbeitsverzeichnis ist %s.", "ディレクトリが存在しません: %s。現在の作業ディレクトリは %s です。", "디렉터리가 존재하지 않습니다: %s. 현재 작업 디렉터리는 %s입니다.", "Каталог не существует: %s. Текущий рабочий каталог: %s.")
	addToolLegacyC(KeyToolLegacyCDidYouMean, " Did you mean %s?", " 是否要使用 %s？", " Meintest du %s?", " %s のことですか？", " %s을(를) 의미하셨나요?", " Возможно, имелось в виду %s?")
	addToolLegacyC(KeyToolLegacyCRipgrepTimedOut, "Ripgrep search timed out", "Ripgrep 搜索超时", "Zeitüberschreitung bei der Ripgrep-Suche", "Ripgrep 検索がタイムアウトしました", "Ripgrep 검색 시간이 초과되었습니다", "Истекло время ожидания поиска Ripgrep")
	addToolLegacyC(KeyToolLegacyCRipgrepRetryFailed, "ripgrep -j 1 retry failed: %s", "ripgrep 使用 -j 1 重试失败：%s", "ripgrep-Wiederholung mit -j 1 fehlgeschlagen: %s", "ripgrep の -j 1 再試行に失敗しました: %s", "ripgrep -j 1 재시도 실패: %s", "Повторный запуск ripgrep с -j 1 завершился ошибкой: %s")
	addToolLegacyC(KeyToolLegacyCRipgrepCriticalError, "ripgrep critical error: %s", "ripgrep 严重错误：%s", "Kritischer ripgrep-Fehler: %s", "ripgrep の重大なエラー: %s", "ripgrep 심각한 오류: %s", "Критическая ошибка ripgrep: %s")
	addToolLegacyC(KeyToolLegacyCRipgrepFailed, "ripgrep failed: %s", "ripgrep 执行失败：%s", "ripgrep fehlgeschlagen: %s", "ripgrep に失敗しました: %s", "ripgrep 실패: %s", "Ошибка ripgrep: %s")

	addToolLegacyC(KeyToolLegacyCEnvNotSet, "environment variable %q not set", "未设置环境变量 %q", "Umgebungsvariable %q ist nicht gesetzt", "環境変数 %q が設定されていません", "환경 변수 %q이(가) 설정되지 않았습니다", "Переменная окружения %q не задана")
	addToolLegacyC(KeyToolLegacyCSetEnvFailed, "failed to set environment variable: %v", "设置环境变量失败：%v", "Umgebungsvariable konnte nicht gesetzt werden: %v", "環境変数を設定できませんでした: %v", "환경 변수를 설정하지 못했습니다: %v", "Не удалось задать переменную окружения: %v")
	addToolLegacyC(KeyToolLegacyCGetCWDFailed, "failed to get current working directory: %v", "获取当前工作目录失败：%v", "Aktuelles Arbeitsverzeichnis konnte nicht ermittelt werden: %v", "現在の作業ディレクトリを取得できませんでした: %v", "현재 작업 디렉터리를 가져오지 못했습니다: %v", "Не удалось получить текущий рабочий каталог: %v")
	addToolLegacyC(KeyToolLegacyCGetWDFailed, "failed to get working directory: %v", "获取工作目录失败：%v", "Arbeitsverzeichnis konnte nicht ermittelt werden: %v", "作業ディレクトリを取得できませんでした: %v", "작업 디렉터리를 가져오지 못했습니다: %v", "Не удалось получить рабочий каталог: %v")
	addToolLegacyC(KeyToolLegacyCResolvePathFailed, "cannot resolve path: %v", "无法解析路径：%v", "Pfad kann nicht aufgelöst werden: %v", "パスを解決できません: %v", "경로를 확인할 수 없습니다: %v", "Не удалось разрешить путь: %v")
	addToolLegacyC(KeyToolLegacyCAccessDirectoryFailed, "cannot access directory: %v", "无法访问目录：%v", "Auf Verzeichnis kann nicht zugegriffen werden: %v", "ディレクトリにアクセスできません: %v", "디렉터리에 접근할 수 없습니다: %v", "Нет доступа к каталогу: %v")
	addToolLegacyC(KeyToolLegacyCPathIsNotDirectory, "path is not a directory: %s", "路径不是目录：%s", "Pfad ist kein Verzeichnis: %s", "パスはディレクトリではありません: %s", "경로가 디렉터리가 아닙니다: %s", "Путь не является каталогом: %s")
	addToolLegacyC(KeyToolLegacyCChangeDirectoryFailed, "failed to change directory: %v", "切换目录失败：%v", "Verzeichnis konnte nicht gewechselt werden: %v", "ディレクトリを変更できませんでした: %v", "디렉터리를 변경하지 못했습니다: %v", "Не удалось сменить каталог: %v")

	addToolLegacyC(KeyToolLegacyCUptimeFailed, "uptime command failed: %v", "uptime 命令执行失败：%v", "uptime-Befehl fehlgeschlagen: %v", "uptime コマンドに失敗しました: %v", "uptime 명령 실패: %v", "Ошибка команды uptime: %v")
	addToolLegacyC(KeyToolLegacyCLastBootUptime, "See lastbootuptime: %s", "请查看 lastbootuptime：%s", "Siehe lastbootuptime: %s", "lastbootuptime を参照してください: %s", "lastbootuptime을 확인하세요: %s", "См. lastbootuptime: %s")
	addToolLegacyC(KeyToolLegacyCProcessListFailed, "process list failed: %v", "获取进程列表失败：%v", "Prozessliste konnte nicht abgerufen werden: %v", "プロセス一覧の取得に失敗しました: %v", "프로세스 목록을 가져오지 못했습니다: %v", "Не удалось получить список процессов: %v")
	addToolLegacyC(KeyToolLegacyCInvalidPID, "invalid PID: %d", "PID 无效：%d", "Ungültige PID: %d", "PID が無効です: %d", "잘못된 PID: %d", "Недопустимый PID: %d")
	addToolLegacyC(KeyToolLegacyCProcessKillFailed, "process kill failed: %v", "终止进程失败：%v", "Prozess konnte nicht beendet werden: %v", "プロセスの終了に失敗しました: %v", "프로세스를 종료하지 못했습니다: %v", "Не удалось завершить процесс: %v")
	addToolLegacyC(KeyToolLegacyCCommandFailed, "command failed: %v", "命令执行失败：%v", "Befehl fehlgeschlagen: %v", "コマンドに失敗しました: %v", "명령 실패: %v", "Ошибка команды: %v")

	addToolLegacyC(KeyToolLegacyCInvalidSendUserMessage, "invalid SendUserMessage input: %v", "SendUserMessage 输入无效：%v", "Ungültige SendUserMessage-Eingabe: %v", "SendUserMessage の入力が無効です: %v", "잘못된 SendUserMessage 입력: %v", "Недопустимый ввод SendUserMessage: %v")
	addToolLegacyC(KeyToolLegacyCInvalidStatus, "invalid status %q: expected one of %q or %q", "状态 %q 无效；应为 %q 或 %q", "Ungültiger Status %q: erwartet wird %q oder %q", "ステータス %q は無効です。%q または %q を指定してください", "잘못된 상태 %q: %q 또는 %q이(가) 필요합니다", "Недопустимый статус %q: ожидался %q или %q")
	addToolLegacyC(KeyToolLegacyCMessageRequired, "message is required", "必须提供 message", "message ist erforderlich", "message は必須です", "message가 필요합니다", "Требуется message")
	addToolLegacyC(KeyToolLegacyCAttachmentsMustBeArray, "invalid SendUserMessage input: attachments must be an array of strings", "SendUserMessage 输入无效：attachments 必须是字符串数组", "Ungültige SendUserMessage-Eingabe: attachments muss ein String-Array sein", "SendUserMessage の入力が無効です: attachments は文字列配列である必要があります", "잘못된 SendUserMessage 입력: attachments는 문자열 배열이어야 합니다", "Недопустимый ввод SendUserMessage: attachments должен быть массивом строк")
	addToolLegacyC(KeyToolLegacyCEncodeSendUserMessageFailed, "encode SendUserMessage output", "编码 SendUserMessage 输出失败", "SendUserMessage-Ausgabe konnte nicht codiert werden", "SendUserMessage の出力をエンコードできませんでした", "SendUserMessage 출력을 인코딩하지 못했습니다", "Не удалось кодировать вывод SendUserMessage")
	addToolLegacyC(KeyToolLegacyCAttachmentMissing, "attachment %q does not exist. Current working directory: %s", "附件 %q 不存在。当前工作目录：%s", "Anhang %q existiert nicht. Aktuelles Arbeitsverzeichnis: %s", "添付ファイル %q が存在しません。現在の作業ディレクトリ: %s", "첨부 파일 %q이(가) 없습니다. 현재 작업 디렉터리: %s", "Вложение %q не существует. Текущий рабочий каталог: %s")
	addToolLegacyC(KeyToolLegacyCAttachmentPermissionDenied, "attachment %q is not accessible (permission denied)", "无法访问附件 %q（权限不足）", "Auf Anhang %q kann nicht zugegriffen werden (Zugriff verweigert)", "添付ファイル %q にアクセスできません（権限がありません）", "첨부 파일 %q에 접근할 수 없습니다(권한 거부)", "Нет доступа к вложению %q (доступ запрещён)")
	addToolLegacyC(KeyToolLegacyCInspectAttachment, "inspect attachment %q", "检查附件 %q 失败", "Anhang %q konnte nicht geprüft werden", "添付ファイル %q を確認できませんでした", "첨부 파일 %q을(를) 검사하지 못했습니다", "Не удалось проверить вложение %q")
	addToolLegacyC(KeyToolLegacyCAttachmentNotRegular, "attachment %q is not a regular file", "附件 %q 不是普通文件", "Anhang %q ist keine reguläre Datei", "添付ファイル %q は通常のファイルではありません", "첨부 파일 %q이(가) 일반 파일이 아닙니다", "Вложение %q не является обычным файлом")
	addToolLegacyC(KeyToolLegacyCOpenAttachment, "open attachment %q", "打开附件 %q 失败", "Anhang %q konnte nicht geöffnet werden", "添付ファイル %q を開けませんでした", "첨부 파일 %q을(를) 열지 못했습니다", "Не удалось открыть вложение %q")
	addToolLegacyC(KeyToolLegacyCCloseAttachment, "close attachment %q", "关闭附件 %q 失败", "Anhang %q konnte nicht geschlossen werden", "添付ファイル %q を閉じられませんでした", "첨부 파일 %q을(를) 닫지 못했습니다", "Не удалось закрыть вложение %q")
	addToolLegacyC(KeyToolLegacyCEmptyWorkingDirectory, "resolve SendUserMessage working directory: empty path", "无法解析 SendUserMessage 工作目录：路径为空", "SendUserMessage-Arbeitsverzeichnis kann nicht aufgelöst werden: leerer Pfad", "SendUserMessage の作業ディレクトリを解決できません: パスが空です", "SendUserMessage 작업 디렉터리를 확인할 수 없습니다: 빈 경로", "Не удалось определить рабочий каталог SendUserMessage: пустой путь")
	addToolLegacyC(KeyToolLegacyCEmptyAttachmentPath, "attachment path must not be empty", "附件路径不能为空", "Anhangspfad darf nicht leer sein", "添付ファイルのパスは空にできません", "첨부 파일 경로는 비워 둘 수 없습니다", "Путь к вложению не должен быть пустым")
	addToolLegacyC(KeyToolLegacyCExpandAttachment, "expand attachment %q", "展开附件路径 %q 失败", "Anhangspfad %q konnte nicht erweitert werden", "添付ファイルのパス %q を展開できませんでした", "첨부 파일 경로 %q을(를) 확장하지 못했습니다", "Не удалось развернуть путь вложения %q")
	addToolLegacyC(KeyToolLegacyCResolveAttachment, "resolve attachment %q", "解析附件路径 %q 失败", "Anhang %q konnte nicht aufgelöst werden", "添付ファイル %q を解決できませんでした", "첨부 파일 %q을(를) 확인하지 못했습니다", "Не удалось разрешить путь вложения %q")
	addToolLegacyC(KeyToolLegacyCMessageDelivered, "Message delivered to user.", "消息已发送给用户。", "Nachricht wurde an den Benutzer gesendet.", "ユーザーにメッセージを送信しました。", "사용자에게 메시지를 보냈습니다.", "Сообщение отправлено пользователю.")
	addToolLegacyC(KeyToolLegacyCOneAttachmentIncluded, " (1 attachment included)", "（已附加 1 个附件）", " (1 Anhang enthalten)", "（添付ファイル1件を含む）", " (첨부 파일 1개 포함)", " (включено 1 вложение)")
	addToolLegacyC(KeyToolLegacyCAttachmentsIncluded, " (%d attachments included)", "（已附加 %d 个附件）", " (%d Anhänge enthalten)", "（添付ファイル%d件を含む）", " (첨부 파일 %d개 포함)", " (включено вложений: %d)")

	addToolLegacyC(KeyToolLegacyCAddressTargetEmpty, "address target must not be empty", "地址目标不能为空", "Adressziel darf nicht leer sein", "アドレスの宛先は空にできません", "주소 대상은 비워 둘 수 없습니다", "Адрес назначения не должен быть пустым")
	addToolLegacyC(KeyToolLegacyCUDSSendFailed, "Failed to send to uds:%s: %s", "发送到 uds:%s 失败：%s", "Senden an uds:%s fehlgeschlagen: %s", "uds:%s への送信に失敗しました: %s", "uds:%s에 보내지 못했습니다: %s", "Не удалось отправить в uds:%s: %s")
	addToolLegacyC(KeyToolLegacyCUDSSent, "“%s” -> uds:%s", "“%s” → uds:%s", "„%s“ -> uds:%s", "「%s」→ uds:%s", "“%s” → uds:%s", "«%s» -> uds:%s")
	addToolLegacyC(KeyToolLegacyCNoBroadcastRecipients, "No teammates to broadcast to (you are the only team member)", "没有可广播的队友（你是团队中唯一的成员）", "Keine Teammitglieder für die Übertragung vorhanden (du bist das einzige Teammitglied)", "ブロードキャスト先のチームメイトがいません（チームメンバーは自分だけです）", "브로드캐스트할 팀원이 없습니다(유일한 팀원입니다)", "Нет участников для рассылки (вы единственный участник команды)")
	addToolLegacyC(KeyToolLegacyCBroadcastSent, "Message broadcast to %d teammate(s): %s", "消息已广播给 %d 名队友：%s", "Nachricht an %d Teammitglied(er) gesendet: %s", "%d人のチームメイトにメッセージをブロードキャストしました: %s", "팀원 %d명에게 메시지를 브로드캐스트했습니다: %s", "Сообщение разослано участникам (%d): %s")
	addToolLegacyC(KeyToolLegacyCInboxSent, "Message sent to %s's inbox", "消息已发送到 %s 的收件箱", "Nachricht an den Posteingang von %s gesendet", "%s の受信トレイにメッセージを送信しました", "%s의 받은 편지함으로 메시지를 보냈습니다", "Сообщение отправлено во входящие %s")
	addToolLegacyC(KeyToolLegacyCShutdownRequestSent, "Shutdown request sent to %s. Request ID: %s", "已向 %s 发送关闭请求。请求 ID：%s", "Beendigungsanfrage an %s gesendet. Anfrage-ID: %s", "%s に終了リクエストを送信しました。リクエスト ID: %s", "%s에게 종료 요청을 보냈습니다. 요청 ID: %s", "Запрос на завершение отправлен %s. ID запроса: %s")
	addToolLegacyC(KeyToolLegacyCShutdownResponseTarget, "shutdown_response must be sent to %q", "shutdown_response 必须发送给 %q", "shutdown_response muss an %q gesendet werden", "shutdown_response は %q に送信する必要があります", "shutdown_response는 %q에게 보내야 합니다", "shutdown_response должен быть отправлен %q")
	addToolLegacyC(KeyToolLegacyCShutdownRequestIDRequired, "shutdown_response requires request_id", "shutdown_response 需要 request_id", "shutdown_response erfordert request_id", "shutdown_response には request_id が必要です", "shutdown_response에는 request_id가 필요합니다", "Для shutdown_response требуется request_id")
	addToolLegacyC(KeyToolLegacyCShutdownApproveRequired, "shutdown_response requires approve", "shutdown_response 需要 approve", "shutdown_response erfordert approve", "shutdown_response には approve が必要です", "shutdown_response에는 approve가 필요합니다", "Для shutdown_response требуется approve")
	addToolLegacyC(KeyToolLegacyCShutdownApproved, "Shutdown approved. Sent confirmation to %s. Agent %s is now exiting.", "关闭请求已批准。已向 %s 发送确认，Agent %s 正在退出。", "Beendigung genehmigt. Bestätigung an %s gesendet. Agent %s wird jetzt beendet.", "終了を承認しました。%s に確認を送信し、Agent %s は終了します。", "종료를 승인했습니다. %s에게 확인을 보냈으며 Agent %s이(가) 종료됩니다.", "Завершение одобрено. Подтверждение отправлено %s. Agent %s завершает работу.")
	addToolLegacyC(KeyToolLegacyCShutdownRejectReason, "reason is required when rejecting a shutdown request", "拒绝关闭请求时必须提供 reason", "Beim Ablehnen einer Beendigungsanfrage ist reason erforderlich", "終了リクエストを拒否する場合は reason が必要です", "종료 요청을 거부할 때 reason이 필요합니다", "При отклонении запроса на завершение требуется reason")
	addToolLegacyC(KeyToolLegacyCShutdownRejected, "Shutdown rejected. Reason: %q. Continuing to work.", "已拒绝关闭请求。原因：%q。将继续工作。", "Beendigung abgelehnt. Grund: %q. Arbeit wird fortgesetzt.", "終了を拒否しました。理由: %q。作業を続行します。", "종료를 거부했습니다. 이유: %q. 작업을 계속합니다.", "Завершение отклонено. Причина: %q. Работа продолжается.")
	addToolLegacyC(KeyToolLegacyCPlanRequestIDRequired, "plan_approval_response requires request_id", "plan_approval_response 需要 request_id", "plan_approval_response erfordert request_id", "plan_approval_response には request_id が必要です", "plan_approval_response에는 request_id가 필요합니다", "Для plan_approval_response требуется request_id")
	addToolLegacyC(KeyToolLegacyCPlanApproveRequired, "plan_approval_response requires approve", "plan_approval_response 需要 approve", "plan_approval_response erfordert approve", "plan_approval_response には approve が必要です", "plan_approval_response에는 approve가 필요합니다", "Для plan_approval_response требуется approve")
	addToolLegacyC(KeyToolLegacyCPlanLeadOnly, "Only the team lead can approve plans. Teammates cannot approve their own or other plans.", "只有 team lead 可以批准计划；队友不能批准自己或他人的计划。", "Nur der Teamleiter kann Pläne genehmigen. Teammitglieder können weder eigene noch fremde Pläne genehmigen.", "計画を承認できるのは team lead だけです。チームメイトは自分や他人の計画を承認できません。", "team lead만 계획을 승인할 수 있습니다. 팀원은 자신이나 다른 팀원의 계획을 승인할 수 없습니다.", "Одобрять планы может только руководитель команды. Участники не могут одобрять свои или чужие планы.")
	addToolLegacyC(KeyToolLegacyCPlanNeedsRevision, "Plan needs revision", "计划需要修改", "Plan muss überarbeitet werden", "計画を修正する必要があります", "계획을 수정해야 합니다", "План требует доработки")
	addToolLegacyC(KeyToolLegacyCPlanApproved, "Plan approved for %s. They will receive the approval and can proceed with implementation.", "已批准 %s 的计划。对方将收到批准通知，可以继续实施。", "Plan für %s genehmigt. Die Person erhält die Genehmigung und kann mit der Umsetzung fortfahren.", "%s の計画を承認しました。承認通知が届き、実装を続行できます。", "%s의 계획을 승인했습니다. 승인 알림을 받은 뒤 구현을 계속할 수 있습니다.", "План для %s одобрен. Участник получит подтверждение и сможет приступить к реализации.")
	addToolLegacyC(KeyToolLegacyCPlanRejected, "Plan rejected for %s with feedback: %q", "%s 的计划已被拒绝，反馈：%q", "Plan für %s mit folgender Rückmeldung abgelehnt: %q", "%s の計画を拒否しました。フィードバック: %q", "%s의 계획을 거부했습니다. 피드백: %q", "План для %s отклонён с комментарием: %q")
	addToolLegacyC(KeyToolLegacyCUnsupportedStructuredType, "unsupported structured message type: %s", "不支持的结构化消息类型：%s", "Nicht unterstützter strukturierter Nachrichtentyp: %s", "未対応の構造化メッセージ種別です: %s", "지원되지 않는 구조화 메시지 유형: %s", "Неподдерживаемый тип структурированного сообщения: %s")
	addToolLegacyC(KeyToolLegacyCMarshalResponseFailed, "failed to marshal response: %v", "序列化响应失败：%v", "Antwort konnte nicht serialisiert werden: %v", "レスポンスをシリアライズできませんでした: %v", "응답을 직렬화하지 못했습니다: %v", "Не удалось сериализовать ответ: %v")
	addToolLegacyC(KeyToolLegacyCEncodeResultFailed, "failed to encode SendMessage result: %v", "编码 SendMessage 结果失败：%v", "SendMessage-Ergebnis konnte nicht codiert werden: %v", "SendMessage の結果をエンコードできませんでした: %v", "SendMessage 결과를 인코딩하지 못했습니다: %v", "Не удалось кодировать результат SendMessage: %v")

	addToolLegacyC(KeyToolLegacyCInputFieldRequired, "Error: invalid input: %s is required", "错误：输入无效：必须提供 %s", "Fehler: Ungültige Eingabe: %s ist erforderlich", "エラー: 入力が無効です: %s は必須です", "오류: 잘못된 입력: %s이(가) 필요합니다", "Ошибка: недопустимый ввод: требуется %s")
	addToolLegacyC(KeyToolLegacyCInputFieldString, "Error: invalid input: %s must be a string", "错误：输入无效：%s 必须是字符串", "Fehler: Ungültige Eingabe: %s muss ein String sein", "エラー: 入力が無効です: %s は文字列である必要があります", "오류: 잘못된 입력: %s은(는) 문자열이어야 합니다", "Ошибка: недопустимый ввод: %s должен быть строкой")
	addToolLegacyC(KeyToolLegacyCInputFieldStringArray, "Error: invalid input: %s must be an array of strings", "错误：输入无效：%s 必须是字符串数组", "Fehler: Ungültige Eingabe: %s muss ein String-Array sein", "エラー: 入力が無効です: %s は文字列配列である必要があります", "오류: 잘못된 입력: %s은(는) 문자열 배열이어야 합니다", "Ошибка: недопустимый ввод: %s должен быть массивом строк")
	addToolLegacyC(KeyToolLegacyCInputMetadataObject, "Error: invalid input: metadata must be an object", "错误：输入无效：metadata 必须是对象", "Fehler: Ungültige Eingabe: metadata muss ein Objekt sein", "エラー: 入力が無効です: metadata はオブジェクトである必要があります", "오류: 잘못된 입력: metadata는 객체여야 합니다", "Ошибка: недопустимый ввод: metadata должен быть объектом")
	addToolLegacyC(KeyToolLegacyCTaskCreated, "Task #%s created successfully: %s", "任务 #%s 创建成功：%s", "Aufgabe #%s erfolgreich erstellt: %s", "タスク #%s を作成しました: %s", "작업 #%s을(를) 만들었습니다: %s", "Задача #%s успешно создана: %s")
	addToolLegacyC(KeyToolLegacyCTaskCreateFailed, "failed to create task: %v", "创建任务失败：%v", "Aufgabe konnte nicht erstellt werden: %v", "タスクを作成できませんでした: %v", "작업을 만들지 못했습니다: %v", "Не удалось создать задачу: %v")
	addToolLegacyC(KeyToolLegacyCTaskCreatedHookFeedback, "TaskCreated hook feedback:\n%s", "TaskCreated hook 反馈：\n%s", "Rückmeldung des TaskCreated-Hooks:\n%s", "TaskCreated hook のフィードバック:\n%s", "TaskCreated hook 피드백:\n%s", "Комментарий hook TaskCreated:\n%s")
	addToolLegacyC(KeyToolLegacyCTaskListInvalidResult, "TaskList returned an invalid typed result", "TaskList 返回了无效的类型化结果", "TaskList gab ein ungültiges typisiertes Ergebnis zurück", "TaskList が無効な型付き結果を返しました", "TaskList가 잘못된 형식의 결과를 반환했습니다", "TaskList вернул недопустимый типизированный результат")
	addToolLegacyC(KeyToolLegacyCNoTasks, "No tasks found", "未找到任务", "Keine Aufgaben gefunden", "タスクが見つかりません", "작업을 찾을 수 없습니다", "Задачи не найдены")
	addToolLegacyC(KeyToolLegacyCTaskBlockedBy, " [blocked by %s]", " [被 %s 阻塞]", " [blockiert durch %s]", " [%s によりブロック中]", " [%s에 의해 차단됨]", " [заблокировано %s]")
	addToolLegacyC(KeyToolLegacyCTaskNotFound, "Task not found", "未找到任务", "Aufgabe nicht gefunden", "タスクが見つかりません", "작업을 찾을 수 없습니다", "Задача не найдена")
	addToolLegacyC(KeyToolLegacyCTaskHeading, "Task #%s: %s", "任务 #%s：%s", "Aufgabe #%s: %s", "タスク #%s: %s", "작업 #%s: %s", "Задача #%s: %s")
	addToolLegacyC(KeyToolLegacyCTaskStatus, "Status: %s", "状态：%s", "Status: %s", "ステータス: %s", "상태: %s", "Статус: %s")
	addToolLegacyC(KeyToolLegacyCTaskDescription, "Description: %s", "说明：%s", "Beschreibung: %s", "説明: %s", "설명: %s", "Описание: %s")
	addToolLegacyC(KeyToolLegacyCTaskGetBlockedBy, "Blocked by: %s", "被以下任务阻塞：%s", "Blockiert durch: %s", "ブロック元: %s", "차단한 작업: %s", "Заблокировано задачами: %s")
	addToolLegacyC(KeyToolLegacyCTaskBlocks, "Blocks: %s", "阻塞以下任务：%s", "Blockiert: %s", "ブロック対象: %s", "차단하는 작업: %s", "Блокирует: %s")
	addToolLegacyC(KeyToolLegacyCInvalidTaskStatus, "invalid status %q; must be pending, in_progress, completed, or deleted", "状态 %q 无效；必须为 pending、in_progress、completed 或 deleted", "Ungültiger Status %q; zulässig sind pending, in_progress, completed oder deleted", "ステータス %q は無効です。pending、in_progress、completed、deleted のいずれかを指定してください", "잘못된 상태 %q: pending, in_progress, completed 또는 deleted여야 합니다", "Недопустимый статус %q; допустимы pending, in_progress, completed или deleted")
	addToolLegacyC(KeyToolLegacyCTaskIDRequired, "taskId is required", "必须提供 taskId", "taskId ist erforderlich", "taskId は必須です", "taskId가 필요합니다", "Требуется taskId")
	addToolLegacyC(KeyToolLegacyCTaskCompletedHookFeedback, "TaskCompleted hook feedback:\n%s", "TaskCompleted hook 反馈：\n%s", "Rückmeldung des TaskCompleted-Hooks:\n%s", "TaskCompleted hook のフィードバック:\n%s", "TaskCompleted hook 피드백:\n%s", "Комментарий hook TaskCompleted:\n%s")
	addToolLegacyC(KeyToolLegacyCBackgroundUnavailable, "background tasks are not available", "后台任务不可用", "Hintergrundaufgaben sind nicht verfügbar", "バックグラウンドタスクは利用できません", "백그라운드 작업을 사용할 수 없습니다", "Фоновые задачи недоступны")
	addToolLegacyC(KeyToolLegacyCBackgroundIDRequired, "Missing required parameter: task_id", "缺少必填参数：task_id", "Erforderlicher Parameter fehlt: task_id", "必須パラメーターがありません: task_id", "필수 매개변수가 없습니다: task_id", "Отсутствует обязательный параметр: task_id")
	addToolLegacyC(KeyToolLegacyCTaskStopped, "Successfully stopped task: %s (%s)", "已成功停止任务：%s（%s）", "Aufgabe erfolgreich beendet: %s (%s)", "タスクを停止しました: %s（%s）", "작업을 중지했습니다: %s (%s)", "Задача успешно остановлена: %s (%s)")
	addToolLegacyC(KeyToolLegacyCBackgroundTaskNotFound, "No task found with ID: %s", "未找到 ID 为 %s 的任务", "Keine Aufgabe mit der ID %s gefunden", "ID %s のタスクが見つかりません", "ID가 %s인 작업을 찾을 수 없습니다", "Задача с ID %s не найдена")
	addToolLegacyC(KeyToolLegacyCReadTaskOutputFailed, "failed to read task output: %v", "读取任务输出失败：%v", "Aufgabenausgabe konnte nicht gelesen werden: %v", "タスク出力を読み取れませんでした: %v", "작업 출력을 읽지 못했습니다: %v", "Не удалось прочитать вывод задачи: %v")
	addToolLegacyC(KeyToolLegacyCTaskOutputTruncated, "[Truncated. Full output: %s]\n\n", "[输出已截断。完整输出：%s]\n\n", "[Gekürzt. Vollständige Ausgabe: %s]\n\n", "[出力は省略されました。完全な出力: %s]\n\n", "[출력이 잘렸습니다. 전체 출력: %s]\n\n", "[Вывод усечён. Полный вывод: %s]\n\n")
	addToolLegacyC(KeyToolLegacyCTaskCreationEmpty, "task creation returned no task", "创建任务后未返回任务", "Aufgabenerstellung gab keine Aufgabe zurück", "タスク作成でタスクが返されませんでした", "작업 생성 결과에 작업이 없습니다", "Создание задачи не вернуло задачу")
	addToolLegacyC(KeyToolLegacyCTaskUpdateFailed, "Task update failed", "任务更新失败", "Aufgabe konnte nicht aktualisiert werden", "タスクの更新に失敗しました", "작업 업데이트 실패", "Не удалось обновить задачу")
	addToolLegacyC(KeyToolLegacyCTaskUpdated, "Updated task #%s%s", "已更新任务 #%s%s", "Aufgabe #%s aktualisiert%s", "タスク #%s を更新しました%s", "작업 #%s 업데이트됨%s", "Задача #%s обновлена%s")
	addToolLegacyC(KeyToolLegacyCTaskUpdatedFields, "Updated task #%s %s%s", "已更新任务 #%s：%s%s", "Aufgabe #%s aktualisiert: %s%s", "タスク #%s を更新しました: %s%s", "작업 #%s 업데이트됨: %s%s", "Задача #%s обновлена: %s%s")
}

func addToolLegacyC(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
