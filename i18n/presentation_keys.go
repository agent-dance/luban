package i18n

// Semantic keys used by the tool-presentation surfaces. Technical identifiers
// such as tool names, protocol names, paths, and raw results are deliberately
// passed as values to these templates rather than translated.
const (
	KeyPresentationToolUpdate           Key = "presentation.tool.update"
	KeyPresentationToolID               Key = "presentation.tool.id"
	KeyPresentationToolUseID            Key = "presentation.tool_use.id"
	KeyPresentationWorkUnit             Key = "presentation.work_unit"
	KeyPresentationActor                Key = "presentation.actor"
	KeyPresentationAction               Key = "presentation.action"
	KeyPresentationObject               Key = "presentation.object"
	KeyPresentationObjectValue          Key = "presentation.object_value"
	KeyPresentationState                Key = "presentation.state"
	KeyPresentationResult               Key = "presentation.result"
	KeyPresentationResultValue          Key = "presentation.result_value"
	KeyPresentationNextAction           Key = "presentation.next_action"
	KeyPresentationDetail               Key = "presentation.detail"
	KeyPresentationDetailsOmitted       Key = "presentation.details_omitted"
	KeyPresentationAdditionalLines      Key = "presentation.additional_lines"
	KeyPresentationLevel                Key = "presentation.level"
	KeyPresentationReasonCodes          Key = "presentation.reason_codes"
	KeyPresentationMoreReasons          Key = "presentation.more_reasons"
	KeyPresentationRedacted             Key = "presentation.redacted"
	KeyPresentationDetailsAvailable     Key = "presentation.details_available"
	KeyPresentationRunning              Key = "presentation.running"
	KeyPresentationAttachment           Key = "presentation.attachment"
	KeyPresentationFile                 Key = "presentation.file"
	KeyPresentationImage                Key = "presentation.image"
	KeyPresentationContextBar           Key = "presentation.context_bar"
	KeyPresentationAggregateRead        Key = "presentation.aggregate.read"
	KeyPresentationAggregateSearch      Key = "presentation.aggregate.search"
	KeyPresentationAggregateWeb         Key = "presentation.aggregate.web"
	KeyPresentationAggregateOperations  Key = "presentation.aggregate.operations"
	KeyPresentationQueued               Key = "presentation.queued"
	KeyPresentationQueueReason          Key = "presentation.queue_reason"
	KeyPresentationReasonUnavailable    Key = "presentation.reason_unavailable"
	KeyPresentationLines                Key = "presentation.lines"
	KeyPresentationWindow               Key = "presentation.window"
	KeyPresentationParts                Key = "presentation.parts"
	KeyPresentationCreated              Key = "presentation.created"
	KeyPresentationUpdated              Key = "presentation.updated"
	KeyPresentationInsertedCell         Key = "presentation.inserted_cell"
	KeyPresentationDeletedCell          Key = "presentation.deleted_cell"
	KeyPresentationReplacedCell         Key = "presentation.replaced_cell"
	KeyPresentationUpdatedCell          Key = "presentation.updated_cell"
	KeyPresentationCell                 Key = "presentation.cell"
	KeyPresentationReplacements         Key = "presentation.replacements"
	KeyPresentationMatches              Key = "presentation.matches"
	KeyPresentationTools                Key = "presentation.tools"
	KeyPresentationFiles                Key = "presentation.files"
	KeyPresentationTruncated            Key = "presentation.truncated"
	KeyPresentationMode                 Key = "presentation.mode"
	KeyPresentationLimit                Key = "presentation.limit"
	KeyPresentationOffset               Key = "presentation.offset"
	KeyPresentationOffsetValue          Key = "presentation.offset_value"
	KeyPresentationExit                 Key = "presentation.exit"
	KeyPresentationBackground           Key = "presentation.background"
	KeyPresentationLatest               Key = "presentation.latest"
	KeyPresentationOutput               Key = "presentation.output"
	KeyPresentationSession              Key = "presentation.session"
	KeyPresentationAgent                Key = "presentation.agent"
	KeyPresentationTokens               Key = "presentation.tokens"
	KeyPresentationInterrupted          Key = "presentation.interrupted"
	KeyPresentationSources              Key = "presentation.sources"
	KeyPresentationResources            Key = "presentation.resources"
	KeyPresentationContents             Key = "presentation.contents"
	KeyPresentationBlob                 Key = "presentation.blob"
	KeyPresentationResultBlocks         Key = "presentation.result_blocks"
	KeyPresentationQuestions            Key = "presentation.questions"
	KeyPresentationChoices              Key = "presentation.choices"
	KeyPresentationAnswers              Key = "presentation.answers"
	KeyPresentationAwaitingApproval     Key = "presentation.awaiting_approval"
	KeyPresentationRequest              Key = "presentation.request"
	KeyPresentationRecipients           Key = "presentation.recipients"
	KeyPresentationAttachments          Key = "presentation.attachments"
	KeyPresentationSent                 Key = "presentation.sent"
	KeyPresentationTarget               Key = "presentation.target"
	KeyPresentationLead                 Key = "presentation.lead"
	KeyPresentationConfig               Key = "presentation.config"
	KeyPresentationMembers              Key = "presentation.members"
	KeyPresentationNext                 Key = "presentation.next"
	KeyPresentationJobs                 Key = "presentation.jobs"
	KeyPresentationRecurring            Key = "presentation.recurring"
	KeyPresentationDurable              Key = "presentation.durable"
	KeyPresentationDiscardedFiles       Key = "presentation.discarded_files"
	KeyPresentationDiscardedCommits     Key = "presentation.discarded_commits"
	KeyPresentationInternalTool         Key = "presentation.internal_tool"
	KeyPresentationUnexpectedProduction Key = "presentation.unexpected_production"
	KeyPresentationOutcome              Key = "presentation.outcome"
	KeyPresentationProcessExit          Key = "presentation.process_exit"
	KeyPresentationDuration             Key = "presentation.duration"
	KeyPresentationTruncatedWarning     Key = "presentation.truncated_warning"
	KeyPresentationImpact               Key = "presentation.impact"
	KeyPresentationReviewNext           Key = "presentation.review_next"
	KeyPresentationTranscript           Key = "presentation.transcript"
	KeyPresentationCause                Key = "presentation.cause"
	KeyPresentationInput                Key = "presentation.input"
	KeyPresentationTasks                Key = "presentation.tasks"
	KeyPresentationBlocked              Key = "presentation.blocked"
	KeyPresentationBlockedBy            Key = "presentation.blocked_by"
	KeyPresentationFields               Key = "presentation.fields"
	KeyPresentationVerificationNeeded   Key = "presentation.verification_needed"
	KeyPresentationOf                   Key = "presentation.of"
	KeyPresentationFollow               Key = "presentation.follow"
	KeyPresentationActive               Key = "presentation.active"
	KeyPresentationPending              Key = "presentation.pending"
	KeyPresentationCompleted            Key = "presentation.completed"
	KeyPresentationBudget               Key = "presentation.budget"
	KeyPresentationFallbackTool         Key = "presentation.fallback_tool"
	KeyPresentationInputKeys            Key = "presentation.input_keys"
	KeyPresentationEvidenceReference    Key = "presentation.evidence_reference"
	KeyPresentationRedactedCommand      Key = "presentation.redacted_command"
	KeyPresentationRedactedDetail       Key = "presentation.redacted_detail"
	KeyPresentationRedactedLocator      Key = "presentation.redacted_locator"
)

func init() {
	for key, translations := range map[Key]map[Language]string{
		KeyPresentationToolUpdate: p("Tool update", "工具更新", "Tool-Aktualisierung", "ツール更新", "도구 업데이트", "Обновление инструмента"),
		KeyPresentationToolID:     p("Tool: %s", "工具：%s", "Tool: %s", "ツール: %s", "도구: %s", "Инструмент: %s"), KeyPresentationToolUseID: p("Tool use ID: %s", "工具调用 ID：%s", "Tool-Aufruf-ID: %s", "ツール使用 ID: %s", "도구 사용 ID: %s", "ID вызова инструмента: %s"), KeyPresentationWorkUnit: p("Work unit: %s", "工作单元：%s", "Arbeitseinheit: %s", "作業単位: %s", "작업 단위: %s", "Единица работы: %s"),
		KeyPresentationActor: p("Actor", "执行者", "Akteur", "実行者", "실행자", "Исполнитель"), KeyPresentationAction: p("Action", "操作", "Aktion", "操作", "작업", "Действие"), KeyPresentationObject: p("Object", "对象", "Objekt", "対象", "对象", "Объект"), KeyPresentationObjectValue: p("Object: %s", "对象：%s", "Objekt: %s", "対象: %s", "对象: %s", "Объект: %s"), KeyPresentationState: p("State", "状态", "Status", "状態", "상태", "Состояние"), KeyPresentationResult: p("Result", "结果", "Ergebnis", "結果", "결과", "Результат"), KeyPresentationResultValue: p("Result: %s", "结果：%s", "Ergebnis: %s", "結果: %s", "결과: %s", "Результат: %s"), KeyPresentationNextAction: p("Next action", "下一步操作", "Nächste Aktion", "次の操作", "다음 작업", "Следующее действие"),
		KeyPresentationDetail: p("Detail %d", "详情 %d", "Detail %d", "詳細 %d", "세부 정보 %d", "Подробность %d"), KeyPresentationDetailsOmitted: p("Details omitted", "已省略详情", "Details ausgelassen", "詳細を省略", "세부 정보 생략됨", "Подробности опущены"), KeyPresentationAdditionalLines: p("%d additional lines", "另有 %d 行", "%d weitere Zeilen", "あと %d 行", "추가 %d줄", "ещё %d строк"), KeyPresentationLevel: p("Presentation level", "展示级别", "Darstellungsebene", "表示レベル", "표시 수준", "Уровень представления"), KeyPresentationReasonCodes: p("Reason codes", "原因代码", "Ursachencodes", "理由コード", "사유 코드", "Коды причин"), KeyPresentationMoreReasons: p(" (+%d more)", "（另有 +%d 项）", " (+%d weitere)", "（ほか %d 件）", "(추가 %d개)", " (+ещё %d)"),
		KeyPresentationRedacted: p("Redacted: sensitive content omitted", "已脱敏：敏感内容已省略", "Geschwärzt: sensible Inhalte ausgelassen", "マスキング済み: 機密内容を省略", "마스킹됨: 민감한 내용 생략", "Скрыто: конфиденциальное содержимое опущено"), KeyPresentationDetailsAvailable: p("Details available", "可查看详情", "Details verfügbar", "詳細あり", "세부 정보 있음", "Подробности доступны"), KeyPresentationRunning: p("%s Running %s... (%.1fs)", "%s 正在运行 %s…（%.1f 秒）", "%s %s läuft … (%.1fs)", "%s %s を実行中… (%.1f秒)", "%s %s 실행 중… (%.1f초)", "%s Выполняется %s… (%.1f с)"),
		KeyPresentationAttachment: p("> [%s] %s (%s)", "> [%s] %s（%s）", "> [%s] %s (%s)", "> [%s] %s（%s）", "> [%s] %s (%s)", "> [%s] %s (%s)"), KeyPresentationFile: p("file", "文件", "Datei", "ファイル", "파일", "файл"), KeyPresentationImage: p("image", "图片", "Bild", "画像", "이미지", "изображение"), KeyPresentationContextBar: p("[%s %d%% (%s/%s)]", "[%s %d%%（%s/%s）]", "[%s %d%% (%s/%s)]", "[%s %d%%（%s/%s）]", "[%s %d%% (%s/%s)]", "[%s %d%% (%s/%s)]"),
		KeyPresentationAggregateRead: p("Read", "读取", "Gelesen", "読み取り", "읽기", "Чтение"), KeyPresentationAggregateSearch: p("Searched", "已搜索", "Gesucht", "検索", "검색", "Поиск"), KeyPresentationAggregateWeb: p("Web", "网页", "Web", "Web", "웹", "Веб"), KeyPresentationAggregateOperations: p("%s · %s operations", "%s · %s 次操作", "%s · %s Vorgänge", "%s · %s 件の操作", "%s · %s개 작업", "%s · %s операций"),
		KeyPresentationQueued: p("%s queued", "%s 已排队", "%s in Warteschlange", "%s をキューに追加", "%s 대기열에 추가됨", "%s в очереди"), KeyPresentationQueueReason: p("Queue reason: %s", "排队原因：%s", "Warteschlangengrund: %s", "キューの理由: %s", "대기열 사유: %s", "Причина очереди: %s"), KeyPresentationReasonUnavailable: p("reason unavailable", "原因不可用", "Grund nicht verfügbar", "理由は不明", "사유를 알 수 없음", "причина недоступна"),
		KeyPresentationLines: p("%d lines", "%d 行", "%d Zeilen", "%d 行", "%d줄", "%d строк"), KeyPresentationWindow: p("window %d..%d/%d", "窗口 %d..%d/%d", "Fenster %d..%d/%d", "範囲 %d..%d/%d", "범위 %d..%d/%d", "окно %d..%d/%d"), KeyPresentationParts: p("%d parts", "%d 部分", "%d Teile", "%d 部", "%d개 부분", "%d частей"), KeyPresentationCreated: p("Created", "已创建", "Erstellt", "作成", "생성됨", "Создано"), KeyPresentationUpdated: p("Updated", "已更新", "Aktualisiert", "更新", "업데이트됨", "Обновлено"),
		KeyPresentationInsertedCell: p("Inserted cell", "已插入单元", "Zelle eingefügt", "セルを挿入", "셀 삽입됨", "Ячейка вставлена"), KeyPresentationDeletedCell: p("Deleted cell", "已删除单元", "Zelle gelöscht", "セルを削除", "셀 삭제됨", "Ячейка удалена"), KeyPresentationReplacedCell: p("Replaced cell", "已替换单元", "Zelle ersetzt", "セルを置換", "셀 교체됨", "Ячейка заменена"), KeyPresentationUpdatedCell: p("Updated cell", "已更新单元", "Zelle aktualisiert", "セルを更新", "셀 업데이트됨", "Ячейка обновлена"), KeyPresentationCell: p("cell %s", "单元 %s", "Zelle %s", "セル %s", "셀 %s", "ячейка %s"), KeyPresentationReplacements: p("%d replacements", "%d 处替换", "%d Ersetzungen", "%d 件置換", "%d개 바꾸기", "%d замен"),
		KeyPresentationMatches: p("%s matches", "%s 个匹配", "%s Treffer", "%s 件一致", "%s개 일치", "%s совпадений"), KeyPresentationTools: p("%s tools", "%s 个工具", "%s Tools", "%s 個のツール", "%s개 도구", "%s инструментов"), KeyPresentationFiles: p("%s files", "%s 个文件", "%s Dateien", "%s ファイル", "%s개 파일", "%s файлов"), KeyPresentationTruncated: p("truncated", "已截断", "gekürzt", "切り詰め", "잘림", "усечено"), KeyPresentationMode: p("mode %s", "模式 %s", "Modus %s", "モード %s", "모드 %s", "режим %s"), KeyPresentationLimit: p("limit %d", "限制 %d", "Limit %d", "上限 %d", "한도 %d", "лимит %d"), KeyPresentationOffset: p("offset %d..%d", "偏移 %d..%d", "Versatz %d..%d", "オフセット %d..%d", "오프셋 %d..%d", "смещение %d..%d"), KeyPresentationOffsetValue: p("offset %d", "偏移 %d", "Versatz %d", "オフセット %d", "오프셋 %d", "смещение %d"),
		KeyPresentationExit: p("exit %s", "退出 %s", "Exit %s", "終了 %s", "종료 %s", "выход %s"), KeyPresentationBackground: p("background %s", "后台 %s", "Hintergrund %s", "バックグラウンド %s", "백그라운드 %s", "фон %s"), KeyPresentationLatest: p("latest %s", "最新 %s", "Neueste: %s", "最新 %s", "최근 %s", "последнее: %s"), KeyPresentationOutput: p("output %s", "输出 %s", "Ausgabe %s", "出力 %s", "출력 %s", "вывод %s"), KeyPresentationSession: p("session %s", "会话 %s", "Sitzung %s", "セッション %s", "세션 %s", "сеанс %s"), KeyPresentationAgent: p("Agent", "Agent", "Agent", "エージェント", "에이전트", "Агент"), KeyPresentationTokens: p("%s tokens", "%s 个令牌", "%s Token", "%s トークン", "%s 토큰", "%s токенов"), KeyPresentationInterrupted: p("interrupted", "已中断", "unterbrochen", "中断", "중단됨", "прервано"),
		KeyPresentationSources: p("%s sources", "%s 个来源", "%s Quellen", "%s 件の情報源", "%s개 소스", "%s источников"), KeyPresentationResources: p("%s resources", "%s 个资源", "%s Ressourcen", "%s リソース", "%s개 리소스", "%s ресурсов"), KeyPresentationContents: p("%d contents", "%d 个内容", "%d Inhalte", "%d 件の内容", "%d개 콘텐츠", "%d содержимого"), KeyPresentationBlob: p("blob %s", "二进制对象 %s", "Blob %s", "blob %s", "blob %s", "blob %s"), KeyPresentationResultBlocks: p("%d result blocks", "%d 个结果块", "%d Ergebnisblöcke", "%d 個の結果ブロック", "%d개 결과 블록", "%d блоков результата"),
		KeyPresentationQuestions: p("%d questions", "%d 个问题", "%d Fragen", "%d 件の質問", "%d개 질문", "%d вопросов"), KeyPresentationChoices: p("%d choices", "%d 个选项", "%d Auswahlmöglichkeiten", "%d 個の選択肢", "%d개 선택지", "%d вариантов"), KeyPresentationAnswers: p("%d answers", "%d 个回答", "%d Antworten", "%d 件の回答", "%d개 답변", "%d ответов"), KeyPresentationAwaitingApproval: p("awaiting leader approval", "等待负责人批准", "Wartet auf Freigabe", "リーダーの承認待ち", "리더 승인 대기 중", "ожидает одобрения руководителя"), KeyPresentationRequest: p("request %s", "请求 %s", "Anfrage %s", "リクエスト %s", "요청 %s", "запрос %s"),
		KeyPresentationRecipients: p("%d recipients", "%d 个收件人", "%d Empfänger", "%d 人の受信者", "%d명 수신자", "%d получателей"), KeyPresentationAttachments: p("%d attachments", "%d 个附件", "%d Anhänge", "%d 件の添付", "%d개 첨부", "%d вложений"), KeyPresentationSent: p("sent %s", "已发送 %s", "gesendet %s", "送信 %s", "전송됨 %s", "отправлено %s"), KeyPresentationTarget: p("to %s", "发送至 %s", "an %s", "宛先 %s", "%s에게", "кому %s"), KeyPresentationLead: p("lead %s", "负责人 %s", "Leitung %s", "リーダー %s", "리더 %s", "руководитель %s"), KeyPresentationConfig: p("config %s", "配置 %s", "Konfiguration %s", "設定 %s", "구성 %s", "конфигурация %s"), KeyPresentationMembers: p("%d members", "%d 名成员", "%d Mitglieder", "%d 名のメンバー", "%d명 구성원", "%d участников"), KeyPresentationNext: p("next %s", "下次 %s", "nächste %s", "次回 %s", "다음 %s", "следующее %s"), KeyPresentationJobs: p("%d jobs", "%d 个任务", "%d Jobs", "%d 件のジョブ", "%d개 작업", "%d заданий"), KeyPresentationRecurring: p("%d recurring", "%d 个循环", "%d wiederkehrend", "%d 件の定期", "%d개 반복", "%d повторяющихся"), KeyPresentationDurable: p("%d durable", "%d 个持久", "%d dauerhaft", "%d 件の永続", "%d개 영구", "%d постоянных"),
		KeyPresentationDiscardedFiles: p("%d files discarded", "已丢弃 %d 个文件", "%d Dateien verworfen", "%d ファイルを破棄", "%d개 파일 폐기", "%d файлов отброшено"), KeyPresentationDiscardedCommits: p("%d commits discarded", "已丢弃 %d 个提交", "%d Commits verworfen", "%d コミットを破棄", "%d개 커밋 폐기", "%d коммитов отброшено"), KeyPresentationInternalTool: p("Internal tool", "内部工具", "Internes Tool", "内部ツール", "내부 도구", "Внутренний инструмент"), KeyPresentationUnexpectedProduction: p("unexpected in production", "不应出现在生产环境", "unerwartet in Produktion", "本番環境では想定外", "프로덕션에서 예기치 않음", "неожиданно в production"),
		KeyPresentationOutcome: p("Outcome: %s", "结果：%s", "Ergebnis: %s", "結果: %s", "결과: %s", "Результат: %s"), KeyPresentationProcessExit: p("Process: exit code %s", "进程：退出码 %s", "Prozess: Exit-Code %s", "プロセス: 終了コード %s", "프로세스: 종료 코드 %s", "Процесс: код выхода %s"), KeyPresentationDuration: p("Duration: %s", "耗时：%s", "Dauer: %s", "所要時間: %s", "소요 시간: %s", "Длительность: %s"), KeyPresentationTruncatedWarning: p("Warning: result was truncated; complete evidence is not claimed", "警告：结果已截断；不声明完整证据可用", "Warnung: Ergebnis wurde gekürzt; vollständige Belege werden nicht zugesichert", "警告: 結果は切り詰められました。完全な証拠があるとは表示しません", "경고: 결과가 잘렸으며 완전한 근거가 있다고 표시하지 않습니다", "Предупреждение: результат усечён; наличие полных данных не заявляется"), KeyPresentationImpact: p("Impact: state-changing operation", "影响：会改变状态的操作", "Auswirkung: zustandsändernde Operation", "影響: 状態を変更する操作", "영향: 상태를 변경하는 작업", "Влияние: операция изменяет состояние"), KeyPresentationReviewNext: p("Next: review the result and retained evidence", "下一步：检查结果和保留的证据", "Nächste Schritte: Ergebnis und Belege prüfen", "次: 結果と保持された証拠を確認", "다음: 결과와 보존된 근거 검토", "Далее: проверить результат и сохранённые данные"), KeyPresentationTranscript: p("Transcript: %s", "记录：%s", "Transkript: %s", "記録: %s", "기록: %s", "Расшифровка: %s"), KeyPresentationCause: p("Cause: %s", "原因：%s", "Ursache: %s", "原因: %s", "원인: %s", "Причина: %s"), KeyPresentationInput: p("Input: %s", "输入：%s", "Eingabe: %s", "入力: %s", "입력: %s", "Ввод: %s"),
		KeyPresentationTasks: p("%d tasks", "%d 个任务", "%d Aufgaben", "%d 件のタスク", "%d개 작업", "%d задач"), KeyPresentationBlocked: p("%d blocked", "%d 个阻塞", "%d blockiert", "%d 件がブロック", "%d개 차단", "%d заблокировано"), KeyPresentationBlockedBy: p("blocked by %d", "%d 项阻塞", "blockiert durch %d", "%d 件によりブロック", "%d개에 의해 차단", "заблокировано: %d"), KeyPresentationFields: p("%d fields", "%d 个字段", "%d Felder", "%d フィールド", "%d개 필드", "%d полей"), KeyPresentationVerificationNeeded: p("verification needed", "需要验证", "Überprüfung erforderlich", "検証が必要", "검증 필요", "требуется проверка"), KeyPresentationOf: p("of %s", "共 %s", "von %s", "%s 中", "%s 중", "из %s"), KeyPresentationFollow: p("follow", "跟随", "folgen", "追従", "따르기", "следовать"), KeyPresentationActive: p("%d active", "%d 个进行中", "%d aktiv", "%d 件が進行中", "%d개 진행 중", "%d активных"), KeyPresentationPending: p("%d pending", "%d 个待处理", "%d ausstehend", "%d 件が保留", "%d개 대기", "%d ожидающих"), KeyPresentationCompleted: p("%d completed", "%d 个已完成", "%d abgeschlossen", "%d 件が完了", "%d개 완료", "%d завершено"), KeyPresentationBudget: p("budget %s", "预算 %s", "Budget %s", "予算 %s", "예산 %s", "бюджет %s"), KeyPresentationFallbackTool: p("Tool", "工具", "Tool", "ツール", "도구", "Инструмент"), KeyPresentationInputKeys: p("input keys: %s", "输入键：%s", "Eingabeschlüssel: %s", "入力キー: %s", "입력 키: %s", "ключи ввода: %s"),
		KeyPresentationEvidenceReference: p("Evidence ref: %s", "证据引用：%s", "Belegreferenz: %s", "証拠参照: %s", "근거 참조: %s", "Ссылка на данные: %s"), KeyPresentationRedactedCommand: p("[REDACTED command]", "[已脱敏命令]", "[GESCHWÄRZTER Befehl]", "[マスキング済みコマンド]", "[마스킹된 명령]", "[СКРЫТАЯ команда]"), KeyPresentationRedactedDetail: p("[REDACTED sensitive detail]", "[已脱敏敏感详情]", "[GESCHWÄRZTES sensibles Detail]", "[マスキング済みの機密詳細]", "[마스킹된 민감한 세부 정보]", "[СКРЫТАЯ конфиденциальная деталь]"), KeyPresentationRedactedLocator: p("[REDACTED locator]", "[已脱敏定位信息]", "[GESCHWÄRZTER Ort]", "[マスキング済みの場所]", "[마스킹된 위치]", "[СКРЫТОЕ расположение]"),
	} {
		semanticTranslations[key] = translations
	}
}

func p(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
