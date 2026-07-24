package i18n

// Semantic copy emitted by the legacy Git, goal, network, plan-mode, rich
// read, and RemoteTrigger tools. Protocol identifiers and raw external values
// are supplied as format arguments and remain untranslated.
const (
	KeyToolGitStatusFailed   Key = "tool.git.status_failed"
	KeyToolGitDiffFailed     Key = "tool.git.diff_failed"
	KeyToolGitLogFailed      Key = "tool.git.log_failed"
	KeyToolGitCommitFailed   Key = "tool.git.commit_failed"
	KeyToolGitPushFailed     Key = "tool.git.push_failed"
	KeyToolGitPullFailed     Key = "tool.git.pull_failed"
	KeyToolGitUnknownAction  Key = "tool.git.unknown_branch_action"
	KeyToolGitBranchFailed   Key = "tool.git.branch_failed"
	KeyToolGitCheckoutFailed Key = "tool.git.checkout_failed"
	KeyToolGitAddFailed      Key = "tool.git.add_failed"
	KeyToolGitCloneFailed    Key = "tool.git.clone_failed"

	KeyToolGoalRuntimeUnavailable  Key = "tool.goal.runtime_unavailable"
	KeyToolGoalLoadFailed          Key = "tool.goal.load_failed"
	KeyToolGoalTokenBudgetPositive Key = "tool.goal.token_budget_positive"
	KeyToolGoalReplaceUnfinished   Key = "tool.goal.replace_unfinished"
	KeyToolGoalSaveFailed          Key = "tool.goal.save_failed"
	KeyToolGoalStatusRequired      Key = "tool.goal.status_required"
	KeyToolGoalNoActive            Key = "tool.goal.no_active"
	KeyToolGoalObjectiveRequired   Key = "tool.goal.objective_required"
	KeyToolGoalObjectiveTooLong    Key = "tool.goal.objective_too_long"
	KeyToolGoalCannotAchieve       Key = "tool.goal.cannot_achieve"
	KeyToolGoalCannotBlock         Key = "tool.goal.cannot_block"
	KeyToolGoalReasonComplete      Key = "tool.goal.reason.complete"
	KeyToolGoalReasonBlocked       Key = "tool.goal.reason.blocked"
	KeyToolGoalInvalidTypedResult  Key = "tool.goal.invalid_typed_result"
	KeyToolGoalNone                Key = "tool.goal.none"
	KeyToolGoalLabelGoal           Key = "tool.goal.label.goal"
	KeyToolGoalLabelStatus         Key = "tool.goal.label.status"
	KeyToolGoalTokenUsageBudget    Key = "tool.goal.token_usage_budget"
	KeyToolGoalTokenUsage          Key = "tool.goal.token_usage"
	KeyToolGoalEvaluatedTurns      Key = "tool.goal.evaluated_turns"
	KeyToolGoalLastEvaluation      Key = "tool.goal.last_evaluation"
	KeyToolGoalCreationEmpty       Key = "tool.goal.creation_empty"
	KeyToolGoalCreated             Key = "tool.goal.created"
	KeyToolGoalUpdateEmpty         Key = "tool.goal.update_empty"
	KeyToolGoalMarked              Key = "tool.goal.marked"
	KeyToolGoalStatusActive        Key = "tool.goal.status.active"
	KeyToolGoalStatusPaused        Key = "tool.goal.status.paused"
	KeyToolGoalStatusAchieved      Key = "tool.goal.status.achieved"
	KeyToolGoalStatusBlocked       Key = "tool.goal.status.blocked"
	KeyToolGoalStatusCleared       Key = "tool.goal.status.cleared"
	KeyToolGoalStatusComplete      Key = "tool.goal.status.complete"
	KeyToolGoalStatusUpdated       Key = "tool.goal.status.updated"

	KeyToolRemoteFeatureResolverFailed Key = "tool.remote_trigger.feature_resolver_failed"
	KeyToolRemoteFeatureDisabled       Key = "tool.remote_trigger.feature_disabled"
	KeyToolRemotePolicyResolverFailed  Key = "tool.remote_trigger.policy_resolver_failed"
	KeyToolRemotePolicyBlocked         Key = "tool.remote_trigger.policy_blocked"

	KeyToolNetworkRequestCreateFailed Key = "tool.network.request_create_failed"
	KeyToolNetworkHTTPRequestFailed   Key = "tool.network.http_request_failed"
	KeyToolNetworkResponseReadFailed  Key = "tool.network.response_read_failed"
	KeyToolNetworkDNSLookupFailed     Key = "tool.network.dns_lookup_failed"
	KeyToolNetworkRecordUnsupported   Key = "tool.network.record_unsupported"
	KeyToolNetworkHostResolveFailed   Key = "tool.network.host_resolve_failed"
	KeyToolNetworkNoIP                Key = "tool.network.no_ip"
	KeyToolNetworkPortInvalid         Key = "tool.network.port_invalid"
	KeyToolNetworkLocalIPFailed       Key = "tool.network.local_ip_failed"
	KeyToolNetworkURLParseFailed      Key = "tool.network.url_parse_failed"
	KeyToolNetworkWHOISConnectFailed  Key = "tool.network.whois_connect_failed"
	KeyToolNetworkWHOISSendFailed     Key = "tool.network.whois_send_failed"
	KeyToolNetworkWHOISReadFailed     Key = "tool.network.whois_read_failed"

	KeyToolPlanRemoteUnavailable Key = "tool.plan.remote_unavailable"
	KeyToolPlanAlreadyActive     Key = "tool.plan.already_active"
	KeyToolPlanInterviewActive   Key = "tool.plan.interview_active"
	KeyToolPlanPrepareFailed     Key = "tool.plan.prepare_failed"
	KeyToolPlanEntered           Key = "tool.plan.entered"

	KeyToolReadTokenLimit         Key = "tool.read.token_limit"
	KeyToolReadSizeLimit          Key = "tool.read.size_limit"
	KeyToolReadBytes              Key = "tool.read.bytes"
	KeyToolReadFileFailed         Key = "tool.read.file_failed"
	KeyToolReadNotebookSummary    Key = "tool.read.notebook_summary"
	KeyToolReadImageSummary       Key = "tool.read.image_summary"
	KeyToolReadPagesInvalid       Key = "tool.read.pages_invalid"
	KeyToolReadPageRangeTooWide   Key = "tool.read.page_range_too_wide"
	KeyToolReadPDFBudget          Key = "tool.read.pdf_budget"
	KeyToolReadPDFTooManyPages    Key = "tool.read.pdf_too_many_pages"
	KeyToolReadPDFFullUnsupported Key = "tool.read.pdf_full_unsupported"

	KeyToolRemoteActionRequired       Key = "tool.remote_trigger.action_required"
	KeyToolRemoteTriggerIDInvalid     Key = "tool.remote_trigger.trigger_id_invalid"
	KeyToolRemoteNotAuthenticated     Key = "tool.remote_trigger.not_authenticated"
	KeyToolRemoteOrganizationMissing  Key = "tool.remote_trigger.organization_missing"
	KeyToolRemoteRequestFailed        Key = "tool.remote_trigger.request_failed"
	KeyToolRemoteEncodeBodyFailed     Key = "tool.remote_trigger.encode_body_failed"
	KeyToolRemoteBuildRequestFailed   Key = "tool.remote_trigger.build_request_failed"
	KeyToolRemoteReadResponseFailed   Key = "tool.remote_trigger.read_response_failed"
	KeyToolRemoteGetNeedsTriggerID    Key = "tool.remote_trigger.get_needs_trigger_id"
	KeyToolRemoteCreateNeedsBody      Key = "tool.remote_trigger.create_needs_body"
	KeyToolRemoteUpdateNeedsTriggerID Key = "tool.remote_trigger.update_needs_trigger_id"
	KeyToolRemoteUpdateNeedsBody      Key = "tool.remote_trigger.update_needs_body"
	KeyToolRemoteRunNeedsTriggerID    Key = "tool.remote_trigger.run_needs_trigger_id"
	KeyToolRemoteActionUnsupported    Key = "tool.remote_trigger.action_unsupported"
	KeyToolRemoteOAuthEndpointInvalid Key = "tool.remote_trigger.oauth_endpoint_invalid"
)

func init() {
	entries := map[Key][6]string{
		KeyToolGitStatusFailed:   {"git status failed: %v", "git status 失败：%v", "git status fehlgeschlagen: %v", "git status に失敗しました: %v", "git status 실패: %v", "Ошибка git status: %v"},
		KeyToolGitDiffFailed:     {"git diff failed: %v", "git diff 失败：%v", "git diff fehlgeschlagen: %v", "git diff に失敗しました: %v", "git diff 실패: %v", "Ошибка git diff: %v"},
		KeyToolGitLogFailed:      {"git log failed: %v", "git log 失败：%v", "git log fehlgeschlagen: %v", "git log に失敗しました: %v", "git log 실패: %v", "Ошибка git log: %v"},
		KeyToolGitCommitFailed:   {"git commit failed: %v", "git commit 失败：%v", "git commit fehlgeschlagen: %v", "git commit に失敗しました: %v", "git commit 실패: %v", "Ошибка git commit: %v"},
		KeyToolGitPushFailed:     {"git push failed: %v", "git push 失败：%v", "git push fehlgeschlagen: %v", "git push に失敗しました: %v", "git push 실패: %v", "Ошибка git push: %v"},
		KeyToolGitPullFailed:     {"git pull failed: %v", "git pull 失败：%v", "git pull fehlgeschlagen: %v", "git pull に失敗しました: %v", "git pull 실패: %v", "Ошибка git pull: %v"},
		KeyToolGitUnknownAction:  {"unknown branch action: %s", "未知的分支操作：%s", "Unbekannte Branch-Aktion: %s", "不明なブランチ操作です: %s", "알 수 없는 브랜치 작업: %s", "Неизвестное действие с веткой: %s"},
		KeyToolGitBranchFailed:   {"git branch failed: %v", "git branch 失败：%v", "git branch fehlgeschlagen: %v", "git branch に失敗しました: %v", "git branch 실패: %v", "Ошибка git branch: %v"},
		KeyToolGitCheckoutFailed: {"git checkout failed: %v", "git checkout 失败：%v", "git checkout fehlgeschlagen: %v", "git checkout に失敗しました: %v", "git checkout 실패: %v", "Ошибка git checkout: %v"},
		KeyToolGitAddFailed:      {"git add failed: %v", "git add 失败：%v", "git add fehlgeschlagen: %v", "git add に失敗しました: %v", "git add 실패: %v", "Ошибка git add: %v"},
		KeyToolGitCloneFailed:    {"git clone failed: %v", "git clone 失败：%v", "git clone fehlgeschlagen: %v", "git clone に失敗しました: %v", "git clone 실패: %v", "Ошибка git clone: %v"},

		KeyToolGoalRuntimeUnavailable:  {"goal runtime is unavailable", "Goal runtime 不可用", "Goal-Runtime ist nicht verfügbar", "Goal runtime を利用できません", "Goal runtime을 사용할 수 없습니다", "Goal runtime недоступна"},
		KeyToolGoalLoadFailed:          {"failed to load goal: %v", "无法加载 Goal：%v", "Goal konnte nicht geladen werden: %v", "Goal を読み込めませんでした: %v", "Goal을 불러오지 못했습니다: %v", "Не удалось загрузить Goal: %v"},
		KeyToolGoalTokenBudgetPositive: {"token_budget must be a positive integer when provided", "提供 token_budget 时必须使用正整数", "token_budget muss, wenn angegeben, eine positive Ganzzahl sein", "token_budget を指定する場合は正の整数にしてください", "token_budget을 지정할 때는 양의 정수여야 합니다", "Если token_budget указан, он должен быть положительным целым числом"},
		KeyToolGoalReplaceUnfinished:   {"cannot replace unfinished goal with status %s", "无法替换状态为 %s 的未完成 Goal", "Unfertiges Goal mit Status %s kann nicht ersetzt werden", "ステータスが %s の未完了 Goal は置き換えられません", "상태가 %s인 완료되지 않은 Goal은 교체할 수 없습니다", "Нельзя заменить незавершённый Goal со статусом %s"},
		KeyToolGoalSaveFailed:          {"failed to save goal: %v", "无法保存 Goal：%v", "Goal konnte nicht gespeichert werden: %v", "Goal を保存できませんでした: %v", "Goal을 저장하지 못했습니다: %v", "Не удалось сохранить Goal: %v"},
		KeyToolGoalStatusRequired:      {"status must be complete, blocked, or revise", "status 必须为 complete、blocked 或 revise", "status muss complete, blocked oder revise sein", "status は complete、blocked、revise のいずれかにしてください", "status는 complete, blocked 또는 revise여야 합니다", "status должен быть complete, blocked или revise"},
		KeyToolGoalNoActive:            {"no active goal to update", "没有可更新的活动 Goal", "Kein aktives Goal zum Aktualisieren", "更新するアクティブな Goal がありません", "업데이트할 활성 Goal이 없습니다", "Нет активного Goal для обновления"},
		KeyToolGoalObjectiveRequired:   {"goal objective is required", "必须提供 Goal 目标", "Ein Goal-Ziel ist erforderlich", "Goal の目標を指定してください", "Goal 목표가 필요합니다", "Необходимо указать цель Goal"},
		KeyToolGoalObjectiveTooLong:    {"goal objective must not exceed 4000 characters", "Goal 目标不得超过 4000 个字符", "Das Goal-Ziel darf höchstens 4000 Zeichen lang sein", "Goal の目標は 4000 文字以内にしてください", "Goal 목표는 4000자를 초과할 수 없습니다", "Цель Goal не должна превышать 4000 символов"},
		KeyToolGoalCannotAchieve:       {"goal: cannot achieve from %s status: invalid transition", "Goal：无法从 %s 状态标记为完成：状态转换无效", "Goal kann aus dem Status %s nicht abgeschlossen werden: ungültiger Übergang", "Goal: ステータス %s から完了にはできません: 無効な遷移", "Goal: %s 상태에서는 완료 처리할 수 없습니다: 잘못된 전환", "Goal: нельзя завершить из статуса %s: недопустимый переход"},
		KeyToolGoalCannotBlock:         {"goal: cannot block from %s status: invalid transition", "Goal：无法从 %s 状态标记为阻塞：状态转换无效", "Goal kann aus dem Status %s nicht blockiert werden: ungültiger Übergang", "Goal: ステータス %s からブロックにはできません: 無効な遷移", "Goal: %s 상태에서는 차단 처리할 수 없습니다: 잘못된 전환", "Goal: нельзя заблокировать из статуса %s: недопустимый переход"},
		KeyToolGoalReasonComplete:      {"marked complete by model", "由模型标记为完成", "Vom Modell als abgeschlossen markiert", "モデルが完了としてマーク", "모델이 완료로 표시함", "Модель отметила как завершённый"},
		KeyToolGoalReasonBlocked:       {"marked blocked by model", "由模型标记为阻塞", "Vom Modell als blockiert markiert", "モデルがブロックとしてマーク", "모델이 차단됨으로 표시함", "Модель отметила как заблокированный"},
		KeyToolGoalInvalidTypedResult:  {"%s returned an invalid typed result", "%s 返回了无效的类型化结果", "%s hat ein ungültiges typisiertes Ergebnis zurückgegeben", "%s が無効な型付き結果を返しました", "%s이(가) 잘못된 형식의 결과를 반환했습니다", "%s вернул недопустимый типизированный результат"},
		KeyToolGoalNone:                {"No goal is set.", "尚未设置 Goal。", "Es ist kein Goal festgelegt.", "Goal は設定されていません。", "설정된 Goal이 없습니다.", "Goal не задан."},
		KeyToolGoalLabelGoal:           {"Goal: %s", "Goal：%s", "Goal: %s", "Goal: %s", "Goal: %s", "Goal: %s"},
		KeyToolGoalLabelStatus:         {"Status: %s", "状态：%s", "Status: %s", "ステータス: %s", "상태: %s", "Статус: %s"},
		KeyToolGoalTokenUsageBudget:    {"Token usage: %d/%d", "Token 用量：%d/%d", "Token-Nutzung: %d/%d", "Token 使用量: %d/%d", "Token 사용량: %d/%d", "Использование Token: %d/%d"},
		KeyToolGoalTokenUsage:          {"Token usage: %d", "Token 用量：%d", "Token-Nutzung: %d", "Token 使用量: %d", "Token 사용량: %d", "Использование Token: %d"},
		KeyToolGoalEvaluatedTurns:      {"Evaluated turns: %d", "已评估轮次：%d", "Ausgewertete Runden: %d", "評価済みターン: %d", "평가된 턴: %d", "Оценено ходов: %d"},
		KeyToolGoalLastEvaluation:      {"Last evaluation: %s", "上次评估：%s", "Letzte Auswertung: %s", "前回の評価: %s", "최근 평가: %s", "Последняя оценка: %s"},
		KeyToolGoalCreationEmpty:       {"Goal creation returned no goal.", "创建 Goal 后未返回 Goal。", "Die Goal-Erstellung hat kein Goal zurückgegeben.", "Goal の作成結果に Goal がありません。", "Goal 생성 결과에 Goal이 없습니다.", "При создании Goal не был возвращён."},
		KeyToolGoalCreated:             {"Goal created: %s", "Goal 已创建：%s", "Goal erstellt: %s", "Goal を作成しました: %s", "Goal 생성됨: %s", "Goal создан: %s"},
		KeyToolGoalUpdateEmpty:         {"Goal update returned no goal.", "更新 Goal 后未返回 Goal。", "Die Goal-Aktualisierung hat kein Goal zurückgegeben.", "Goal の更新結果に Goal がありません。", "Goal 업데이트 결과에 Goal이 없습니다.", "При обновлении Goal не был возвращён."},
		KeyToolGoalMarked:              {"Goal marked %s: %s", "Goal 已标记为%s：%s", "Goal als %s markiert: %s", "Goal を%sとしてマークしました: %s", "Goal을 %s 상태로 표시함: %s", "Goal отмечен как %s: %s"},
		KeyToolGoalStatusActive:        {"active", "进行中", "aktiv", "進行中", "진행 중", "активный"},
		KeyToolGoalStatusPaused:        {"paused", "已暂停", "pausiert", "一時停止", "일시 중지됨", "приостановлен"},
		KeyToolGoalStatusAchieved:      {"achieved", "已达成", "erreicht", "達成済み", "달성됨", "достигнут"},
		KeyToolGoalStatusBlocked:       {"blocked", "已阻塞", "blockiert", "ブロック中", "차단됨", "заблокирован"},
		KeyToolGoalStatusCleared:       {"cleared", "已清除", "gelöscht", "クリア済み", "지워짐", "очищен"},
		KeyToolGoalStatusComplete:      {"complete", "完成", "abgeschlossen", "完了", "완료", "завершён"},
		KeyToolGoalStatusUpdated:       {"updated", "更新", "aktualisiert", "更新済み", "업데이트됨", "обновлён"},

		KeyToolRemoteFeatureResolverFailed: {"feature flag resolver error: %v", "feature flag 解析器出错：%v", "Fehler im Feature-Flag-Resolver: %v", "feature flag resolver エラー: %v", "feature flag resolver 오류: %v", "Ошибка resolver feature flag: %v"},
		KeyToolRemoteFeatureDisabled:       {"RemoteTrigger is disabled by feature flag tengu_surreal_dali.", "RemoteTrigger 已被 feature flag tengu_surreal_dali 禁用。", "RemoteTrigger ist durch das Feature-Flag tengu_surreal_dali deaktiviert.", "RemoteTrigger は feature flag tengu_surreal_dali により無効です。", "RemoteTrigger가 feature flag tengu_surreal_dali에 의해 비활성화되었습니다.", "RemoteTrigger отключён feature flag tengu_surreal_dali."},
		KeyToolRemotePolicyResolverFailed:  {"policy resolver error: %v", "policy 解析器出错：%v", "Fehler im Policy-Resolver: %v", "policy resolver エラー: %v", "policy resolver 오류: %v", "Ошибка resolver policy: %v"},
		KeyToolRemotePolicyBlocked:         {"RemoteTrigger blocked by policy: allow_remote_sessions=false.", "RemoteTrigger 已被 policy 阻止：allow_remote_sessions=false。", "RemoteTrigger wurde durch die Policy blockiert: allow_remote_sessions=false.", "RemoteTrigger は policy によりブロックされました: allow_remote_sessions=false。", "RemoteTrigger가 policy에 의해 차단되었습니다: allow_remote_sessions=false.", "RemoteTrigger заблокирован policy: allow_remote_sessions=false."},

		KeyToolNetworkRequestCreateFailed: {"failed to create request: %v", "无法创建请求：%v", "Anfrage konnte nicht erstellt werden: %v", "リクエストを作成できませんでした: %v", "요청을 생성하지 못했습니다: %v", "Не удалось создать запрос: %v"},
		KeyToolNetworkHTTPRequestFailed:   {"HTTP request failed: %v", "HTTP 请求失败：%v", "HTTP-Anfrage fehlgeschlagen: %v", "HTTP リクエストに失敗しました: %v", "HTTP 요청 실패: %v", "Ошибка HTTP-запроса: %v"},
		KeyToolNetworkResponseReadFailed:  {"failed to read response: %v", "无法读取响应：%v", "Antwort konnte nicht gelesen werden: %v", "レスポンスを読み取れませんでした: %v", "응답을 읽지 못했습니다: %v", "Не удалось прочитать ответ: %v"},
		KeyToolNetworkDNSLookupFailed:     {"DNS lookup failed: %v", "DNS 查询失败：%v", "DNS-Abfrage fehlgeschlagen: %v", "DNS 検索に失敗しました: %v", "DNS 조회 실패: %v", "Ошибка DNS-поиска: %v"},
		KeyToolNetworkRecordUnsupported:   {"unsupported record type: %s", "不支持的记录类型：%s", "Nicht unterstützter Eintragstyp: %s", "未対応のレコード種別です: %s", "지원하지 않는 레코드 유형: %s", "Неподдерживаемый тип записи: %s"},
		KeyToolNetworkHostResolveFailed:   {"host resolution failed: %v", "主机解析失败：%v", "Host-Auflösung fehlgeschlagen: %v", "ホストの名前解決に失敗しました: %v", "호스트 확인 실패: %v", "Ошибка разрешения имени хоста: %v"},
		KeyToolNetworkNoIP:                {"no IP addresses found for host: %s", "未找到主机的 IP 地址：%s", "Keine IP-Adressen für den Host gefunden: %s", "ホストの IP アドレスが見つかりません: %s", "호스트의 IP 주소를 찾지 못했습니다: %s", "Для хоста не найдены IP-адреса: %s"},
		KeyToolNetworkPortInvalid:         {"invalid port number: %d", "端口号无效：%d", "Ungültige Portnummer: %d", "ポート番号が無効です: %d", "잘못된 포트 번호: %d", "Недопустимый номер порта: %d"},
		KeyToolNetworkLocalIPFailed:       {"failed to get local IP: %v", "无法获取本机 IP：%v", "Lokale IP konnte nicht ermittelt werden: %v", "ローカル IP を取得できませんでした: %v", "로컬 IP를 가져오지 못했습니다: %v", "Не удалось получить локальный IP: %v"},
		KeyToolNetworkURLParseFailed:      {"failed to parse URL: %v", "无法解析 URL：%v", "URL konnte nicht geparst werden: %v", "URL を解析できませんでした: %v", "URL을 파싱하지 못했습니다: %v", "Не удалось разобрать URL: %v"},
		KeyToolNetworkWHOISConnectFailed:  {"WHOIS server connection failed: %v", "连接 WHOIS 服务器失败：%v", "Verbindung zum WHOIS-Server fehlgeschlagen: %v", "WHOIS サーバーへの接続に失敗しました: %v", "WHOIS 서버 연결 실패: %v", "Не удалось подключиться к серверу WHOIS: %v"},
		KeyToolNetworkWHOISSendFailed:     {"failed to send WHOIS query: %v", "无法发送 WHOIS 查询：%v", "WHOIS-Anfrage konnte nicht gesendet werden: %v", "WHOIS クエリを送信できませんでした: %v", "WHOIS 쿼리를 보내지 못했습니다: %v", "Не удалось отправить WHOIS-запрос: %v"},
		KeyToolNetworkWHOISReadFailed:     {"failed to read WHOIS response: %v", "无法读取 WHOIS 响应：%v", "WHOIS-Antwort konnte nicht gelesen werden: %v", "WHOIS レスポンスを読み取れませんでした: %v", "WHOIS 응답을 읽지 못했습니다: %v", "Не удалось прочитать ответ WHOIS: %v"},

		KeyToolPlanRemoteUnavailable: {"Error: EnterPlanMode is unavailable in remote-channel mode because plan approval requires the terminal.", "错误：remote-channel 模式下无法使用 EnterPlanMode，因为计划审批需要终端。", "Fehler: EnterPlanMode ist im remote-channel-Modus nicht verfügbar, da die Plangenehmigung das Terminal benötigt.", "エラー: plan の承認にはターミナルが必要なため、remote-channel mode では EnterPlanMode を使用できません。", "오류: plan 승인에 터미널이 필요하므로 remote-channel mode에서는 EnterPlanMode를 사용할 수 없습니다.", "Ошибка: EnterPlanMode недоступен в режиме remote-channel, поскольку для утверждения плана требуется терминал."},
		KeyToolPlanAlreadyActive:     {"already in plan mode", "已处于 plan mode", "Bereits im plan mode", "すでに plan mode です", "이미 plan mode입니다", "Уже в plan mode"},
		KeyToolPlanInterviewActive:   {"Cannot enter plan mode while interview phase %q is active. Complete the interview first, then call EnterPlanMode.", "interview 阶段 %q 正在进行，无法进入 plan mode。请先完成 interview，再调用 EnterPlanMode。", "Der plan mode kann nicht gestartet werden, solange die Interviewphase %q aktiv ist. Schließe zuerst das Interview ab und rufe dann EnterPlanMode auf.", "interview フェーズ %q が進行中のため plan mode には入れません。先に interview を完了してから EnterPlanMode を呼び出してください。", "interview 단계 %q이(가) 진행 중이므로 plan mode에 진입할 수 없습니다. interview를 먼저 완료한 다음 EnterPlanMode를 호출하세요.", "Нельзя войти в plan mode, пока активна фаза interview %q. Сначала завершите interview, затем вызовите EnterPlanMode."},
		KeyToolPlanPrepareFailed:     {"failed to prepare plan mode: %v", "无法准备 plan mode：%v", "plan mode konnte nicht vorbereitet werden: %v", "plan mode を準備できませんでした: %v", "plan mode를 준비하지 못했습니다: %v", "Не удалось подготовить plan mode: %v"},
		KeyToolPlanEntered:           {"Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach.", "已进入 plan mode。现在应专注于探索代码库并设计实现方案。", "plan mode aktiviert. Konzentriere dich jetzt darauf, die Codebasis zu erkunden und einen Implementierungsansatz zu entwerfen.", "plan mode に入りました。ここからはコードベースの調査と実装方針の設計に集中してください。", "plan mode에 진입했습니다. 이제 코드베이스를 탐색하고 구현 방식을 설계하는 데 집중하세요.", "Вход в plan mode выполнен. Теперь сосредоточьтесь на изучении кодовой базы и разработке подхода к реализации."},

		KeyToolReadTokenLimit:         {"File content (%d tokens) exceeds maximum allowed tokens (%d). Use offset and limit parameters to read specific portions of the file, or search for specific content instead of reading the whole file.", "文件内容（%d 个 token）超过允许的 token 上限（%d）。请使用 offset 和 limit 参数读取文件的特定部分，或搜索特定内容，而不要读取整个文件。", "Der Dateiinhalt (%d Token) überschreitet die maximal zulässige Token-Zahl (%d). Verwende offset und limit, um bestimmte Teile zu lesen, oder suche gezielt nach Inhalten, statt die gesamte Datei zu lesen.", "ファイルの内容（%d token）が上限（%d）を超えています。ファイル全体を読む代わりに、offset と limit で必要な部分を読むか、特定の内容を検索してください。", "파일 내용(%d token)이 허용된 최대 token 수(%d)를 초과합니다. 전체 파일을 읽는 대신 offset과 limit로 필요한 부분을 읽거나 특정 내용을 검색하세요.", "Содержимое файла (%d Token) превышает допустимый предел (%d). Используйте offset и limit для чтения нужных частей или ищите конкретное содержимое вместо чтения всего файла."},
		KeyToolReadSizeLimit:          {"File content (%s) exceeds maximum allowed size (%s). Use offset and limit parameters to read specific portions of the file, or search for specific content instead of reading the whole file.", "文件内容（%s）超过允许的大小上限（%s）。请使用 offset 和 limit 参数读取文件的特定部分，或搜索特定内容，而不要读取整个文件。", "Der Dateiinhalt (%s) überschreitet die maximal zulässige Größe (%s). Verwende offset und limit, um bestimmte Teile zu lesen, oder suche gezielt nach Inhalten, statt die gesamte Datei zu lesen.", "ファイルの内容（%s）がサイズ上限（%s）を超えています。ファイル全体を読む代わりに、offset と limit で必要な部分を読むか、特定の内容を検索してください。", "파일 내용(%s)이 허용된 최대 크기(%s)를 초과합니다. 전체 파일을 읽는 대신 offset과 limit로 필요한 부분을 읽거나 특정 내용을 검색하세요.", "Содержимое файла (%s) превышает допустимый размер (%s). Используйте offset и limit для чтения нужных частей или ищите конкретное содержимое вместо чтения всего файла."},
		KeyToolReadBytes:              {"%d bytes", "%d 字节", "%d Bytes", "%d バイト", "%d바이트", "%d байт"},
		KeyToolReadFileFailed:         {"failed to read file: %v", "无法读取文件：%v", "Datei konnte nicht gelesen werden: %v", "ファイルを読み取れませんでした: %v", "파일을 읽지 못했습니다: %v", "Не удалось прочитать файл: %v"},
		KeyToolReadNotebookSummary:    {"Notebook file read: %s (%d cell(s))", "已读取 Notebook 文件：%s（%d 个 cell）", "Notebook-Datei gelesen: %s (%d Zelle(n))", "Notebook ファイルを読み取りました: %s（%d cell）", "Notebook 파일을 읽었습니다: %s(%d개 cell)", "Файл Notebook прочитан: %s (ячеек: %d)"},
		KeyToolReadImageSummary:       {"Image file read: %s (%s)", "已读取图像文件：%s（%s）", "Bilddatei gelesen: %s (%s)", "画像ファイルを読み取りました: %s（%s）", "이미지 파일을 읽었습니다: %s(%s)", "Файл изображения прочитан: %s (%s)"},
		KeyToolReadPagesInvalid:       {"Invalid pages parameter: %q. Use formats like \"1-5\", \"3\", or \"10-20\". Pages are 1-indexed.", "pages 参数无效：%q。请使用 \"1-5\"、\"3\" 或 \"10-20\" 等格式。页码从 1 开始。", "Ungültiger pages-Parameter: %q. Verwende Formate wie \"1-5\", \"3\" oder \"10-20\". Seiten werden ab 1 gezählt.", "pages パラメータが無効です: %q。\"1-5\"、\"3\"、\"10-20\" のような形式を使用してください。ページ番号は 1 から始まります。", "잘못된 pages 매개변수: %q. \"1-5\", \"3\", \"10-20\"과 같은 형식을 사용하세요. 페이지 번호는 1부터 시작합니다.", "Недопустимый параметр pages: %q. Используйте формат \"1-5\", \"3\" или \"10-20\". Нумерация страниц начинается с 1."},
		KeyToolReadPageRangeTooWide:   {"Page range %q exceeds maximum of %d pages per request. Please use a smaller range.", "页码范围 %q 超过每次请求最多 %d 页的限制。请缩小范围。", "Der Seitenbereich %q überschreitet das Maximum von %d Seiten pro Anfrage. Verwende einen kleineren Bereich.", "ページ範囲 %q は 1 回のリクエスト上限 %d ページを超えています。範囲を狭めてください。", "페이지 범위 %q이(가) 요청당 최대 %d페이지를 초과합니다. 더 작은 범위를 사용하세요.", "Диапазон %q превышает максимум %d страниц на запрос. Уменьшите диапазон."},
		KeyToolReadPDFBudget:          {"Selected PDF pages would consume ~%d tokens, exceeding the remaining budget (%d). Reduce the page range.", "所选 PDF 页面约需 %d 个 token，超过剩余预算（%d）。请缩小页码范围。", "Die ausgewählten PDF-Seiten würden etwa %d Token verbrauchen und das verbleibende Budget (%d) überschreiten. Verkleinere den Seitenbereich.", "選択した PDF ページは約 %d token を消費し、残りの予算（%d）を超えます。ページ範囲を狭めてください。", "선택한 PDF 페이지는 약 %d token을 사용하여 남은 예산(%d)을 초과합니다. 페이지 범위를 줄이세요.", "Выбранные страницы PDF потребуют около %d Token, что превышает оставшийся бюджет (%d). Уменьшите диапазон."},
		KeyToolReadPDFTooManyPages:    {"This PDF has %d pages, which is too many to read at once. Use the pages parameter to read specific page ranges (e.g., pages: \"1-5\"). Maximum %d pages per request.", "此 PDF 共 %d 页，无法一次性读取。请使用 pages 参数读取指定页码范围（例如 pages: \"1-5\"）。每次请求最多 %d 页。", "Dieses PDF hat %d Seiten und kann nicht auf einmal gelesen werden. Verwende den pages-Parameter für bestimmte Bereiche (z. B. pages: \"1-5\"). Pro Anfrage sind höchstens %d Seiten möglich.", "この PDF は %d ページあるため、一度には読み取れません。pages パラメータで範囲を指定してください（例: pages: \"1-5\"）。1 回につき最大 %d ページです。", "이 PDF는 %d페이지이므로 한 번에 읽기에는 너무 큽니다. pages 매개변수로 범위를 지정하세요(예: pages: \"1-5\"). 요청당 최대 %d페이지입니다.", "В этом PDF %d страниц — слишком много для чтения за один раз. Укажите диапазон через pages (например, pages: \"1-5\"). Максимум %d страниц на запрос."},
		KeyToolReadPDFFullUnsupported: {"Reading full PDFs is not supported with this model. Use a newer model (Sonnet 3.5 v2 or later), or use the pages parameter to read specific page ranges (e.g., pages: \"1-5\", maximum %d pages per request). Page extraction requires poppler-utils: install with `brew install poppler` on macOS or `apt-get install poppler-utils` on Debian/Ubuntu.", "此模型不支持读取完整 PDF。请使用较新的模型（Sonnet 3.5 v2 或更高版本），或使用 pages 参数读取指定页码范围（例如 pages: \"1-5\"，每次请求最多 %d 页）。页面提取需要 poppler-utils：macOS 上运行 `brew install poppler`，Debian/Ubuntu 上运行 `apt-get install poppler-utils`。", "Dieses Modell unterstützt nicht das Lesen vollständiger PDFs. Verwende ein neueres Modell (Sonnet 3.5 v2 oder höher) oder den pages-Parameter für bestimmte Bereiche (z. B. pages: \"1-5\", maximal %d Seiten pro Anfrage). Für die Seitenextraktion wird poppler-utils benötigt: unter macOS mit `brew install poppler`, unter Debian/Ubuntu mit `apt-get install poppler-utils` installieren.", "このモデルでは PDF 全体の読み取りはサポートされていません。新しいモデル（Sonnet 3.5 v2 以降）を使用するか、pages パラメータでページ範囲を指定してください（例: pages: \"1-5\"、1 回につき最大 %d ページ）。ページ抽出には poppler-utils が必要です。macOS では `brew install poppler`、Debian/Ubuntu では `apt-get install poppler-utils` を実行してください。", "이 모델은 전체 PDF 읽기를 지원하지 않습니다. 최신 모델(Sonnet 3.5 v2 이상)을 사용하거나 pages 매개변수로 범위를 지정하세요(예: pages: \"1-5\", 요청당 최대 %d페이지). 페이지 추출에는 poppler-utils가 필요합니다. macOS에서는 `brew install poppler`, Debian/Ubuntu에서는 `apt-get install poppler-utils`를 실행하세요.", "Эта модель не поддерживает чтение PDF целиком. Используйте более новую модель (Sonnet 3.5 v2 или новее) либо задайте диапазон через pages (например, pages: \"1-5\", максимум %d страниц на запрос). Для извлечения страниц требуется poppler-utils: установите через `brew install poppler` в macOS или `apt-get install poppler-utils` в Debian/Ubuntu."},

		KeyToolRemoteActionRequired:       {"action is required", "必须提供 action", "action ist erforderlich", "action は必須です", "action이 필요합니다", "Необходимо указать action"},
		KeyToolRemoteTriggerIDInvalid:     {"trigger_id must match ^[\\w-]+$", "trigger_id 必须匹配 ^[\\w-]+$", "trigger_id muss ^[\\w-]+$ entsprechen", "trigger_id は ^[\\w-]+$ に一致する必要があります", "trigger_id는 ^[\\w-]+$와 일치해야 합니다", "trigger_id должен соответствовать ^[\\w-]+$"},
		KeyToolRemoteNotAuthenticated:     {"Not authenticated with a claude.ai account. Run /login and try again.", "尚未通过 claude.ai 账户认证。请运行 /login 后重试。", "Nicht mit einem claude.ai-Konto authentifiziert. Führe /login aus und versuche es erneut.", "claude.ai アカウントで認証されていません。/login を実行してから再試行してください。", "claude.ai 계정으로 인증되지 않았습니다. /login을 실행한 후 다시 시도하세요.", "Нет аутентификации с учётной записью claude.ai. Выполните /login и повторите попытку."},
		KeyToolRemoteOrganizationMissing:  {"Unable to resolve organization UUID.", "无法解析组织 UUID。", "Die Organisations-UUID konnte nicht ermittelt werden.", "組織 UUID を解決できませんでした。", "조직 UUID를 확인할 수 없습니다.", "Не удалось определить UUID организации."},
		KeyToolRemoteRequestFailed:        {"remote trigger request failed: %v", "remote trigger 请求失败：%v", "Remote-Trigger-Anfrage fehlgeschlagen: %v", "remote trigger リクエストに失敗しました: %v", "remote trigger 요청 실패: %v", "Ошибка запроса remote trigger: %v"},
		KeyToolRemoteEncodeBodyFailed:     {"failed to encode request body: %v", "无法编码请求体：%v", "Anfragetext konnte nicht codiert werden: %v", "リクエスト本文をエンコードできませんでした: %v", "요청 본문을 인코딩하지 못했습니다: %v", "Не удалось закодировать тело запроса: %v"},
		KeyToolRemoteBuildRequestFailed:   {"failed to build request: %v", "无法构建请求：%v", "Anfrage konnte nicht erstellt werden: %v", "リクエストを構築できませんでした: %v", "요청을 구성하지 못했습니다: %v", "Не удалось сформировать запрос: %v"},
		KeyToolRemoteReadResponseFailed:   {"failed to read response: %v", "无法读取响应：%v", "Antwort konnte nicht gelesen werden: %v", "レスポンスを読み取れませんでした: %v", "응답을 읽지 못했습니다: %v", "Не удалось прочитать ответ: %v"},
		KeyToolRemoteGetNeedsTriggerID:    {"get requires trigger_id", "get 需要 trigger_id", "get erfordert trigger_id", "get には trigger_id が必要です", "get에는 trigger_id가 필요합니다", "Для get требуется trigger_id"},
		KeyToolRemoteCreateNeedsBody:      {"create requires body", "create 需要 body", "create erfordert body", "create には body が必要です", "create에는 body가 필요합니다", "Для create требуется body"},
		KeyToolRemoteUpdateNeedsTriggerID: {"update requires trigger_id", "update 需要 trigger_id", "update erfordert trigger_id", "update には trigger_id が必要です", "update에는 trigger_id가 필요합니다", "Для update требуется trigger_id"},
		KeyToolRemoteUpdateNeedsBody:      {"update requires body", "update 需要 body", "update erfordert body", "update には body が必要です", "update에는 body가 필요합니다", "Для update требуется body"},
		KeyToolRemoteRunNeedsTriggerID:    {"run requires trigger_id", "run 需要 trigger_id", "run erfordert trigger_id", "run には trigger_id が必要です", "run에는 trigger_id가 필요합니다", "Для run требуется trigger_id"},
		KeyToolRemoteActionUnsupported:    {"unsupported action: %s", "不支持的 action：%s", "Nicht unterstützte action: %s", "未対応の action です: %s", "지원하지 않는 action: %s", "Неподдерживаемая action: %s"},
		KeyToolRemoteOAuthEndpointInvalid: {"CLAUDE_CODE_CUSTOM_OAUTH_URL is not an approved endpoint", "CLAUDE_CODE_CUSTOM_OAUTH_URL 不是获准的 endpoint", "CLAUDE_CODE_CUSTOM_OAUTH_URL ist kein zugelassener Endpoint", "CLAUDE_CODE_CUSTOM_OAUTH_URL は承認済み endpoint ではありません", "CLAUDE_CODE_CUSTOM_OAUTH_URL은 승인된 endpoint가 아닙니다", "CLAUDE_CODE_CUSTOM_OAUTH_URL не является разрешённым endpoint"},
	}
	for key, values := range entries {
		semanticTranslations[key] = map[Language]string{
			LangEN: values[0], LangZH: values[1], LangDE: values[2],
			LangJA: values[3], LangKO: values[4], LangRU: values[5],
		}
	}
}
