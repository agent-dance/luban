package i18n

// Semantic copy for schedule tools and their persistence lifecycle. Cron
// expressions, prompts, job and owner identifiers, limits, schema versions,
// and raw external causes remain format arguments and are not translated.
const (
	KeyToolScheduleCreateDescription Key = "tool.schedule.create.description"
	KeyToolScheduleDeleteDescription Key = "tool.schedule.delete.description"
	KeyToolScheduleListDescription   Key = "tool.schedule.list.description"

	KeyToolScheduleSchemaCron      Key = "tool.schedule.schema.cron"
	KeyToolScheduleSchemaPrompt    Key = "tool.schedule.schema.prompt"
	KeyToolScheduleSchemaRecurring Key = "tool.schedule.schema.recurring"
	KeyToolScheduleSchemaDurable   Key = "tool.schedule.schema.durable"
	KeyToolScheduleSchemaID        Key = "tool.schedule.schema.id"

	KeyToolScheduleInvalidInput         Key = "tool.schedule.error.invalid_input"
	KeyToolScheduleInvalidExpression    Key = "tool.schedule.error.invalid_expression"
	KeyToolScheduleNoFutureFire         Key = "tool.schedule.error.no_future_fire"
	KeyToolScheduleTooMany              Key = "tool.schedule.error.too_many"
	KeyToolScheduleDurableUnavailable   Key = "tool.schedule.error.durable_unavailable"
	KeyToolScheduleAgentDenied          Key = "tool.schedule.error.agent_denied"
	KeyToolScheduleStoreReadFailed      Key = "tool.schedule.error.store_read_failed"
	KeyToolScheduleStoreWriteFailed     Key = "tool.schedule.error.store_write_failed"
	KeyToolScheduleStoreCorrupt         Key = "tool.schedule.error.store_corrupt"
	KeyToolScheduleStoreVersion         Key = "tool.schedule.error.store_version"
	KeyToolScheduleStoreSecurity        Key = "tool.schedule.error.store_security"
	KeyToolScheduleLeaderUnavailable    Key = "tool.schedule.error.leader_unavailable"
	KeyToolScheduleStartFailed          Key = "tool.schedule.error.start_failed"
	KeyToolScheduleExecutorUnavailable  Key = "tool.schedule.error.executor_unavailable"
	KeyToolScheduleEnqueueFailed        Key = "tool.schedule.error.enqueue_failed"
	KeyToolScheduleAckFailed            Key = "tool.schedule.error.ack_failed"
	KeyToolScheduleInvalidTypedResult   Key = "tool.schedule.error.invalid_typed_result"
	KeyToolScheduleRandomIDFailed       Key = "tool.schedule.error.random_id_failed"
	KeyToolScheduleStopFailed           Key = "tool.schedule.error.stop_failed"
	KeyToolScheduleExecutionDescription Key = "tool.schedule.execution.description"

	KeyToolScheduleCreatedRecurring Key = "tool.schedule.created.recurring"
	KeyToolScheduleCreatedOneShot   Key = "tool.schedule.created.one_shot"
	KeyToolScheduleCreatedSession   Key = "tool.schedule.created.session"
	KeyToolScheduleCreatedPersisted Key = "tool.schedule.created.persisted"
	KeyToolScheduleCancelled        Key = "tool.schedule.cancelled"
	KeyToolScheduleNotFound         Key = "tool.schedule.not_found"
	KeyToolScheduleOwnerDenied      Key = "tool.schedule.owner_denied"
	KeyToolScheduleNoJobs           Key = "tool.schedule.list.empty"
	KeyToolScheduleListRow          Key = "tool.schedule.list.row"
	KeyToolScheduleNextFireUnknown  Key = "tool.schedule.next_fire.unknown"
)

var toolScheduleKeys = [...]Key{
	KeyToolScheduleCreateDescription,
	KeyToolScheduleDeleteDescription,
	KeyToolScheduleListDescription,
	KeyToolScheduleSchemaCron,
	KeyToolScheduleSchemaPrompt,
	KeyToolScheduleSchemaRecurring,
	KeyToolScheduleSchemaDurable,
	KeyToolScheduleSchemaID,
	KeyToolScheduleInvalidInput,
	KeyToolScheduleInvalidExpression,
	KeyToolScheduleNoFutureFire,
	KeyToolScheduleTooMany,
	KeyToolScheduleDurableUnavailable,
	KeyToolScheduleAgentDenied,
	KeyToolScheduleStoreReadFailed,
	KeyToolScheduleStoreWriteFailed,
	KeyToolScheduleStoreCorrupt,
	KeyToolScheduleStoreVersion,
	KeyToolScheduleStoreSecurity,
	KeyToolScheduleLeaderUnavailable,
	KeyToolScheduleStartFailed,
	KeyToolScheduleExecutorUnavailable,
	KeyToolScheduleEnqueueFailed,
	KeyToolScheduleAckFailed,
	KeyToolScheduleInvalidTypedResult,
	KeyToolScheduleRandomIDFailed,
	KeyToolScheduleStopFailed,
	KeyToolScheduleExecutionDescription,
	KeyToolScheduleCreatedRecurring,
	KeyToolScheduleCreatedOneShot,
	KeyToolScheduleCreatedSession,
	KeyToolScheduleCreatedPersisted,
	KeyToolScheduleCancelled,
	KeyToolScheduleNotFound,
	KeyToolScheduleOwnerDenied,
	KeyToolScheduleNoJobs,
	KeyToolScheduleListRow,
	KeyToolScheduleNextFireUnknown,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolScheduleCreateDescription,
		"Create a scheduled job from a cron expression and prompt.",
		"根据 cron 表达式和提示词创建计划任务。",
		"Erstellt aus einem Cron-Ausdruck und einer Anweisung eine geplante Aufgabe.",
		"cron 式とプロンプトからスケジュール済みジョブを作成します。",
		"cron 식과 프롬프트로 예약 작업을 만듭니다.",
		"Создаёт запланированную задачу по cron-выражению и запросу.")
	add(KeyToolScheduleDeleteDescription,
		"Cancel a scheduled job by ID.",
		"按 ID 取消计划任务。",
		"Bricht eine geplante Aufgabe anhand ihrer ID ab.",
		"ID を指定してスケジュール済みジョブをキャンセルします。",
		"ID로 예약 작업을 취소합니다.",
		"Отменяет запланированную задачу по идентификатору.")
	add(KeyToolScheduleListDescription,
		"List scheduled jobs owned by this session.",
		"列出当前会话拥有的计划任务。",
		"Listet die geplanten Aufgaben dieser Sitzung auf.",
		"このセッションが所有するスケジュール済みジョブを一覧表示します。",
		"이 세션이 소유한 예약 작업을 나열합니다.",
		"Показывает запланированные задачи, принадлежащие этому сеансу.")

	add(KeyToolScheduleSchemaCron,
		"Cron expression that determines when the job runs.",
		"用于确定任务运行时间的 cron 表达式。",
		"Cron-Ausdruck, der die Ausführungszeit der Aufgabe bestimmt.",
		"ジョブの実行時刻を決める cron 式。",
		"작업 실행 시점을 정하는 cron 식입니다.",
		"Cron-выражение, определяющее время запуска задачи.")
	add(KeyToolScheduleSchemaPrompt,
		"Prompt to run when the schedule fires.",
		"计划触发时要运行的提示词。",
		"Anweisung, die beim Auslösen des Zeitplans ausgeführt wird.",
		"スケジュールの発火時に実行するプロンプト。",
		"일정이 시작될 때 실행할 프롬프트입니다.",
		"Запрос, выполняемый при срабатывании расписания.")
	add(KeyToolScheduleSchemaRecurring,
		"Whether the job should continue after its first successful delivery.",
		"任务在首次成功投递后是否继续运行。",
		"Gibt an, ob die Aufgabe nach der ersten erfolgreichen Zustellung fortgesetzt wird.",
		"最初の配信成功後もジョブを継続するかどうか。",
		"첫 번째 전달 성공 후에도 작업을 계속할지 여부입니다.",
		"Продолжать ли задачу после первой успешной доставки.")
	add(KeyToolScheduleSchemaDurable,
		"Whether the job should persist in .luban-code/schedule/jobs.json.",
		"是否将任务持久化到 .luban-code/schedule/jobs.json。",
		"Gibt an, ob die Aufgabe in .luban-code/schedule/jobs.json gespeichert wird.",
		"ジョブを .luban-code/schedule/jobs.json に永続化するかどうか。",
		"작업을 .luban-code/schedule/jobs.json에 영구 저장할지 여부입니다.",
		"Сохранять ли задачу в .luban-code/schedule/jobs.json.")
	add(KeyToolScheduleSchemaID,
		"ID of the scheduled job.",
		"计划任务的 ID。",
		"ID der geplanten Aufgabe.",
		"スケジュール済みジョブの ID。",
		"예약 작업의 ID입니다.",
		"Идентификатор запланированной задачи.")

	add(KeyToolScheduleInvalidInput,
		"Invalid schedule input: %v",
		"计划任务输入无效：%v",
		"Ungültige Zeitplan-Eingabe: %v",
		"スケジュールの入力が無効です: %v",
		"일정 입력이 올바르지 않습니다: %v",
		"Недопустимые параметры расписания: %v")
	add(KeyToolScheduleInvalidExpression,
		"Invalid cron expression %q: %v",
		"cron 表达式 %q 无效：%v",
		"Ungültiger Cron-Ausdruck %q: %v",
		"cron 式 %q が無効です: %v",
		"cron 식 %q이(가) 올바르지 않습니다: %v",
		"Недопустимое cron-выражение %q: %v")
	add(KeyToolScheduleNoFutureFire,
		"Cron expression %q has no future run time.",
		"cron 表达式 %q 没有未来的运行时间。",
		"Der Cron-Ausdruck %q hat keinen zukünftigen Ausführungszeitpunkt.",
		"cron 式 %q には今後の実行時刻がありません。",
		"cron 식 %q에 향후 실행 시점이 없습니다.",
		"Для cron-выражения %q нет будущего времени запуска.")
	add(KeyToolScheduleTooMany,
		"Scheduled job limit reached (%d). Cancel a job before creating another.",
		"计划任务已达到上限（%d 个）。请先取消一个任务。",
		"Das Limit für geplante Aufgaben ist erreicht (%d). Brich eine Aufgabe ab, bevor du eine weitere erstellst.",
		"スケジュール済みジョブの上限（%d 件）に達しました。新規作成の前にジョブをキャンセルしてください。",
		"예약 작업 한도(%d개)에 도달했습니다. 새 작업을 만들기 전에 기존 작업을 취소하세요.",
		"Достигнут предел запланированных задач (%d). Перед созданием новой задачи отмените одну из существующих.")
	add(KeyToolScheduleDurableUnavailable,
		"Persistent schedules are unavailable in this runtime.",
		"当前运行环境不支持持久计划任务。",
		"Persistente Zeitpläne sind in dieser Laufzeit nicht verfügbar.",
		"このランタイムでは永続スケジュールを利用できません。",
		"이 런타임에서는 영구 일정을 사용할 수 없습니다.",
		"Постоянные расписания недоступны в этой среде выполнения.")
	add(KeyToolScheduleAgentDenied,
		"This agent is not allowed to create persistent schedules.",
		"此 Agent 无权创建持久计划任务。",
		"Dieser Agent darf keine persistenten Zeitpläne erstellen.",
		"この Agent には永続スケジュールを作成する権限がありません。",
		"이 Agent에는 영구 일정을 만들 권한이 없습니다.",
		"Этому Agent запрещено создавать постоянные расписания.")
	add(KeyToolScheduleStoreReadFailed,
		"Could not read .luban-code/schedule/jobs.json: %v",
		"无法读取 .luban-code/schedule/jobs.json：%v",
		".luban-code/schedule/jobs.json konnte nicht gelesen werden: %v",
		".luban-code/schedule/jobs.json を読み込めませんでした: %v",
		".luban-code/schedule/jobs.json을 읽을 수 없습니다: %v",
		"Не удалось прочитать .luban-code/schedule/jobs.json: %v")
	add(KeyToolScheduleStoreWriteFailed,
		"Could not write .luban-code/schedule/jobs.json: %v",
		"无法写入 .luban-code/schedule/jobs.json：%v",
		".luban-code/schedule/jobs.json konnte nicht geschrieben werden: %v",
		".luban-code/schedule/jobs.json に書き込めませんでした: %v",
		".luban-code/schedule/jobs.json에 쓸 수 없습니다: %v",
		"Не удалось записать .luban-code/schedule/jobs.json: %v")
	add(KeyToolScheduleStoreCorrupt,
		"Schedule data in .luban-code/schedule/jobs.json is invalid: %v",
		".luban-code/schedule/jobs.json 中的计划任务数据无效：%v",
		"Die Zeitplandaten in .luban-code/schedule/jobs.json sind ungültig: %v",
		".luban-code/schedule/jobs.json のスケジュールデータが無効です: %v",
		".luban-code/schedule/jobs.json의 일정 데이터가 올바르지 않습니다: %v",
		"Данные расписания в .luban-code/schedule/jobs.json недопустимы: %v")
	add(KeyToolScheduleStoreVersion,
		"Schedule data version %d is not supported.",
		"不支持版本为 %d 的计划任务数据。",
		"Version %d der Zeitplandaten wird nicht unterstützt.",
		"スケジュールデータのバージョン %d はサポートされていません。",
		"일정 데이터 버전 %d은(는) 지원되지 않습니다.",
		"Версия %d данных расписания не поддерживается.")
	add(KeyToolScheduleStoreSecurity,
		"Schedule storage failed its security check: %v",
		"计划任务存储未通过安全检查：%v",
		"Der Zeitplanspeicher hat die Sicherheitsprüfung nicht bestanden: %v",
		"スケジュールストレージのセキュリティチェックに失敗しました: %v",
		"일정 저장소가 보안 검사를 통과하지 못했습니다: %v",
		"Хранилище расписаний не прошло проверку безопасности: %v")
	add(KeyToolScheduleLeaderUnavailable,
		"A schedule leader is not available yet; try again shortly.",
		"计划任务 leader 尚不可用，请稍后重试。",
		"Noch ist kein Zeitplan-Leader verfügbar; versuche es gleich noch einmal.",
		"スケジュールの leader はまだ利用できません。しばらくしてから再試行してください。",
		"일정 leader를 아직 사용할 수 없습니다. 잠시 후 다시 시도하세요.",
		"Ведущий процесс расписания пока недоступен; повторите попытку чуть позже.")
	add(KeyToolScheduleStartFailed,
		"Could not start the schedule service: %v",
		"无法启动计划任务服务：%v",
		"Der Zeitplandienst konnte nicht gestartet werden: %v",
		"スケジュールサービスを開始できませんでした: %v",
		"일정 서비스를 시작할 수 없습니다: %v",
		"Не удалось запустить службу расписаний: %v")
	add(KeyToolScheduleExecutorUnavailable,
		"The schedule executor is unavailable.",
		"计划任务执行器不可用。",
		"Die Zeitplanausführung ist nicht verfügbar.",
		"スケジュール実行機能を利用できません。",
		"일정 실행기를 사용할 수 없습니다.",
		"Исполнитель расписаний недоступен.")
	add(KeyToolScheduleEnqueueFailed,
		"Could not queue scheduled job %s: %v",
		"无法将计划任务 %s 加入队列：%v",
		"Die geplante Aufgabe %s konnte nicht eingereiht werden: %v",
		"スケジュール済みジョブ %s をキューに追加できませんでした: %v",
		"예약 작업 %s을(를) 대기열에 추가할 수 없습니다: %v",
		"Не удалось поставить запланированную задачу %s в очередь: %v")
	add(KeyToolScheduleAckFailed,
		"Scheduled job %s ran, but its delivery could not be recorded: %v",
		"计划任务 %s 已运行，但无法记录其投递结果：%v",
		"Die geplante Aufgabe %s wurde ausgeführt, ihre Zustellung konnte jedoch nicht bestätigt werden: %v",
		"スケジュール済みジョブ %s は実行されましたが、配信結果を記録できませんでした: %v",
		"예약 작업 %s이(가) 실행되었지만 전달 결과를 기록할 수 없습니다: %v",
		"Запланированная задача %s выполнена, но результат доставки не удалось записать: %v")
	add(KeyToolScheduleInvalidTypedResult,
		"The schedule operation returned an invalid result: %v",
		"计划任务操作返回了无效结果：%v",
		"Der Zeitplanvorgang hat ein ungültiges Ergebnis zurückgegeben: %v",
		"スケジュール操作から無効な結果が返されました: %v",
		"일정 작업이 올바르지 않은 결과를 반환했습니다: %v",
		"Операция с расписанием вернула недопустимый результат: %v")
	add(KeyToolScheduleRandomIDFailed,
		"Could not generate a secure scheduled-job ID.",
		"无法生成安全的计划任务 ID。",
		"Es konnte keine sichere ID für die geplante Aufgabe erzeugt werden.",
		"安全なスケジュール済みジョブ ID を生成できませんでした。",
		"안전한 예약 작업 ID를 생성할 수 없습니다.",
		"Не удалось создать безопасный идентификатор запланированной задачи.")
	add(KeyToolScheduleStopFailed,
		"Could not stop the schedule service: %v",
		"无法停止计划任务服务：%v",
		"Der Zeitplandienst konnte nicht beendet werden: %v",
		"スケジュールサービスを停止できませんでした: %v",
		"일정 서비스를 중지할 수 없습니다: %v",
		"Не удалось остановить службу расписаний: %v")
	add(KeyToolScheduleExecutionDescription,
		"Run scheduled job %s.",
		"运行计划任务 %s。",
		"Geplante Aufgabe %s ausführen.",
		"スケジュール済みジョブ %s を実行します。",
		"예약 작업 %s을(를) 실행합니다.",
		"Выполнить запланированную задачу %s.")

	add(KeyToolScheduleCreatedRecurring,
		"Created recurring scheduled job %s; next run: %s.",
		"已创建周期计划任务 %s；下次运行时间：%s。",
		"Wiederkehrende geplante Aufgabe %s erstellt; nächste Ausführung: %s.",
		"繰り返し実行するスケジュール済みジョブ %s を作成しました。次回の実行: %s。",
		"반복 예약 작업 %s을(를) 만들었습니다. 다음 실행: %s.",
		"Создана повторяющаяся задача %s; следующий запуск: %s.")
	add(KeyToolScheduleCreatedOneShot,
		"Created one-time scheduled job %s; run time: %s.",
		"已创建单次计划任务 %s；运行时间：%s。",
		"Einmalige geplante Aufgabe %s erstellt; Ausführung: %s.",
		"一度だけ実行するスケジュール済みジョブ %s を作成しました。実行時刻: %s。",
		"일회성 예약 작업 %s을(를) 만들었습니다. 실행 시각: %s.",
		"Создана однократная задача %s; время запуска: %s.")
	add(KeyToolScheduleCreatedSession,
		"Job %s is active for this session only.",
		"任务 %s 仅在当前会话中有效。",
		"Aufgabe %s ist nur für diese Sitzung aktiv.",
		"ジョブ %s はこのセッション中のみ有効です。",
		"작업 %s은(는) 이 세션에서만 유효합니다.",
		"Задача %s действует только в этом сеансе.")
	add(KeyToolScheduleCreatedPersisted,
		"Job %s was saved to .luban-code/schedule/jobs.json.",
		"任务 %s 已保存到 .luban-code/schedule/jobs.json。",
		"Aufgabe %s wurde in .luban-code/schedule/jobs.json gespeichert.",
		"ジョブ %s を .luban-code/schedule/jobs.json に保存しました。",
		"작업 %s을(를) .luban-code/schedule/jobs.json에 저장했습니다.",
		"Задача %s сохранена в .luban-code/schedule/jobs.json.")
	add(KeyToolScheduleCancelled,
		"Canceled scheduled job %s.",
		"已取消计划任务 %s。",
		"Geplante Aufgabe %s abgebrochen.",
		"スケジュール済みジョブ %s をキャンセルしました。",
		"예약 작업 %s을(를) 취소했습니다.",
		"Запланированная задача %s отменена.")
	add(KeyToolScheduleNotFound,
		"Scheduled job %s was not found.",
		"未找到计划任务 %s。",
		"Die geplante Aufgabe %s wurde nicht gefunden.",
		"スケジュール済みジョブ %s が見つかりません。",
		"예약 작업 %s을(를) 찾을 수 없습니다.",
		"Запланированная задача %s не найдена.")
	add(KeyToolScheduleOwnerDenied,
		"Scheduled job %s belongs to another session.",
		"计划任务 %s 属于其他会话。",
		"Die geplante Aufgabe %s gehört zu einer anderen Sitzung.",
		"スケジュール済みジョブ %s は別のセッションに属しています。",
		"예약 작업 %s은(는) 다른 세션에 속합니다.",
		"Запланированная задача %s принадлежит другому сеансу.")
	add(KeyToolScheduleNoJobs,
		"No scheduled jobs.",
		"没有计划任务。",
		"Keine geplanten Aufgaben.",
		"スケジュール済みジョブはありません。",
		"예약 작업이 없습니다.",
		"Нет запланированных задач.")
	add(KeyToolScheduleListRow,
		"%s | %s | next: %s | %s",
		"%s | %s | 下次：%s | %s",
		"%s | %s | nächste Ausführung: %s | %s",
		"%s | %s | 次回: %s | %s",
		"%s | %s | 다음: %s | %s",
		"%s | %s | следующий запуск: %s | %s")
	add(KeyToolScheduleNextFireUnknown,
		"unknown",
		"未知",
		"unbekannt",
		"不明",
		"알 수 없음",
		"неизвестно")
}
