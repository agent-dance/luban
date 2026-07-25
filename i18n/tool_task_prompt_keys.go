package i18n

// Semantic prompt copy for structured task tools and background-task controls.
// Tool and field names, task identifiers, task types, statuses, timeouts, and
// raw task output remain protocol values and are not translated.
const (
	KeyToolTaskCreateDescription                  Key = "tool.task_create.description"
	KeyToolTaskCreateInputSubjectDescription      Key = "tool.task_create.input.subject.description"
	KeyToolTaskCreateInputDescriptionDescription  Key = "tool.task_create.input.description.description"
	KeyToolTaskInputActiveFormDescription         Key = "tool.task.input.active_form.description"
	KeyToolTaskCreateInputMetadataDescription     Key = "tool.task_create.input.metadata.description"
	KeyToolTaskCreateDiscoveryHint                Key = "tool.task_create.discovery_hint"
	KeyToolTaskListDescription                    Key = "tool.task_list.description"
	KeyToolTaskListDiscoveryHint                  Key = "tool.task_list.discovery_hint"
	KeyToolTaskUpdateDescription                  Key = "tool.task_update.description"
	KeyToolTaskUpdateInputTaskIDDescription       Key = "tool.task_update.input.task_id.description"
	KeyToolTaskUpdateInputSubjectDescription      Key = "tool.task_update.input.subject.description"
	KeyToolTaskUpdateInputDescriptionDescription  Key = "tool.task_update.input.description.description"
	KeyToolTaskUpdateInputStatusDescription       Key = "tool.task_update.input.status.description"
	KeyToolTaskUpdateInputAddBlocksDescription    Key = "tool.task_update.input.add_blocks.description"
	KeyToolTaskUpdateInputAddBlockedByDescription Key = "tool.task_update.input.add_blocked_by.description"
	KeyToolTaskUpdateInputOwnerDescription        Key = "tool.task_update.input.owner.description"
	KeyToolTaskUpdateInputMetadataDescription     Key = "tool.task_update.input.metadata.description"
	KeyToolTaskUpdateDiscoveryHint                Key = "tool.task_update.discovery_hint"
	KeyToolTaskGetDescription                     Key = "tool.task_get.description"
	KeyToolTaskGetInputTaskIDDescription          Key = "tool.task_get.input.task_id.description"
	KeyToolTaskGetDiscoveryHint                   Key = "tool.task_get.discovery_hint"
	KeyToolTaskStopDescription                    Key = "tool.task_stop.description"
	KeyToolTaskStopInputTaskIDDescription         Key = "tool.task_stop.input.task_id.description"
	KeyToolTaskOutputDescription                  Key = "tool.task_output.description"
	KeyToolTaskOutputInputTaskIDDescription       Key = "tool.task_output.input.task_id.description"
	KeyToolTaskOutputInputBlockDescription        Key = "tool.task_output.input.block.description"
	KeyToolTaskOutputInputTimeoutDescription      Key = "tool.task_output.input.timeout.description"
)

var toolTaskPromptKeys = [...]Key{
	KeyToolTaskCreateDescription,
	KeyToolTaskCreateInputSubjectDescription,
	KeyToolTaskCreateInputDescriptionDescription,
	KeyToolTaskInputActiveFormDescription,
	KeyToolTaskCreateInputMetadataDescription,
	KeyToolTaskCreateDiscoveryHint,
	KeyToolTaskListDescription,
	KeyToolTaskListDiscoveryHint,
	KeyToolTaskUpdateDescription,
	KeyToolTaskUpdateInputTaskIDDescription,
	KeyToolTaskUpdateInputSubjectDescription,
	KeyToolTaskUpdateInputDescriptionDescription,
	KeyToolTaskUpdateInputStatusDescription,
	KeyToolTaskUpdateInputAddBlocksDescription,
	KeyToolTaskUpdateInputAddBlockedByDescription,
	KeyToolTaskUpdateInputOwnerDescription,
	KeyToolTaskUpdateInputMetadataDescription,
	KeyToolTaskUpdateDiscoveryHint,
	KeyToolTaskGetDescription,
	KeyToolTaskGetInputTaskIDDescription,
	KeyToolTaskGetDiscoveryHint,
	KeyToolTaskStopDescription,
	KeyToolTaskStopInputTaskIDDescription,
	KeyToolTaskOutputDescription,
	KeyToolTaskOutputInputTaskIDDescription,
	KeyToolTaskOutputInputBlockDescription,
	KeyToolTaskOutputInputTimeoutDescription,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolTaskCreateDescription,
		`Create a new task in the task list

Use this tool to create a structured task list for the current coding session. It helps track progress, organize complex tasks, and show overall progress to the user.

## When to Use This Tool

- Use it for complex work with three or more distinct steps, non-trivial planning, plan mode, explicit todo-list requests, or multiple user tasks.
- Capture new requirements as tasks, mark a task in_progress before work, and mark it completed as soon as it is done.

## When NOT to Use This Tool

- Skip it for one straightforward or trivial task, work completed in fewer than three trivial steps, and purely conversational requests.

## Task Fields

- subject: a brief actionable title in imperative form.
- description: enough detail to complete the work, including enough context for teammates when team mode is active.
- activeForm: optional present-continuous text shown while the task is in progress.

All new tasks start pending with no owner. Use TaskUpdate to assign teammates and establish dependencies through blocks/blockedBy. Check TaskList first to avoid duplicate tasks.`,
		`在任务列表中创建新任务

使用此工具为当前编码会话创建结构化任务列表。它有助于跟踪进度、组织复杂工作，并向用户展示整体进展。

## 何时使用此工具

- 工作包含三个或更多不同步骤、需要实质性规划、处于计划模式、用户明确要求待办列表，或用户同时提出多个任务时使用。
- 将新需求记录为任务；开始工作前将任务标记为 in_progress，完成后立即标记为 completed。

## 何时不使用此工具

- 对单个直接或简单的任务、能在少于三个简单步骤中完成的工作，以及纯对话请求，不要使用此工具。

## 任务字段

- subject：使用祈使形式的简短、可执行标题。
- description：完成工作所需的充分细节；启用团队模式时，还应为队友提供足够上下文。
- activeForm：可选的进行时文本，在任务进行中显示。

所有新任务初始状态均为 pending 且没有 owner。使用 TaskUpdate 分配队友，并通过 blocks/blockedBy 建立依赖关系。请先检查 TaskList，避免创建重复任务。`,
		`Eine neue Aufgabe in der Aufgabenliste erstellen

Mit diesem Tool wird eine strukturierte Aufgabenliste für die aktuelle Programmiersitzung erstellt. Sie hilft, den Fortschritt zu verfolgen, komplexe Arbeiten zu organisieren und den Gesamtfortschritt anzuzeigen.

## Wann dieses Tool verwendet werden sollte

- Für komplexe Arbeiten mit mindestens drei unterschiedlichen Schritten, wesentliche Planung, den Planungsmodus, ausdrücklich angeforderte Aufgabenlisten oder mehrere Benutzeraufgaben.
- Neue Anforderungen als Aufgaben erfassen, eine Aufgabe vor Arbeitsbeginn als in_progress markieren und sie unmittelbar nach Abschluss als completed markieren.

## Wann dieses Tool nicht verwendet werden sollte

- Nicht für eine einzelne einfache oder triviale Aufgabe, Arbeiten mit weniger als drei trivialen Schritten oder reine Gesprächsanfragen verwenden.

## Aufgabenfelder

- subject: ein kurzer, ausführbarer Titel im Imperativ.
- description: ausreichende Details zur Durchführung; im Teammodus auch genügend Kontext für Teammitglieder.
- activeForm: optionaler Verlaufsform-Text, der angezeigt wird, solange die Aufgabe bearbeitet wird.

Alle neuen Aufgaben beginnen ohne owner im Status pending. Mit TaskUpdate können Teammitglieder zugewiesen und Abhängigkeiten über blocks/blockedBy festgelegt werden. Zuerst TaskList prüfen, um doppelte Aufgaben zu vermeiden.`,
		`タスクリストに新しいタスクを作成

このツールは、現在のコーディングセッション用に構造化されたタスクリストを作成します。進捗の追跡、複雑な作業の整理、ユーザーへの全体的な進捗表示に役立ちます。

## このツールを使用する場合

- 3 つ以上の異なる手順を含む複雑な作業、実質的な計画、プランモード、明示的な Todo リストの依頼、または複数のユーザータスクに使用します。
- 新しい要件をタスクとして記録し、作業開始前にタスクを in_progress に、完了後すぐに completed にします。

## このツールを使用しない場合

- 1 つの単純または些細なタスク、3 つ未満の些細な手順で完了する作業、純粋な会話の依頼には使用しません。

## タスクフィールド

- subject: 命令形で記述した、短く実行可能なタイトル。
- description: 作業の完了に十分な詳細。チームモードが有効な場合は、チームメイトに必要なコンテキストも含めます。
- activeForm: タスクの進行中に表示する、省略可能な進行形テキスト。

新しいタスクはすべて owner なしの pending で始まります。TaskUpdate でチームメイトを割り当て、blocks/blockedBy で依存関係を設定します。重複を避けるため、最初に TaskList を確認してください。`,
		`작업 목록에 새 작업 만들기

이 도구는 현재 코딩 세션을 위한 구조화된 작업 목록을 만듭니다. 진행 상황을 추적하고 복잡한 작업을 정리하며 사용자에게 전체 진행 상황을 보여 주는 데 도움이 됩니다.

## 이 도구를 사용할 때

- 서로 다른 단계가 세 개 이상인 복잡한 작업, 실질적인 계획, 계획 모드, 명시적인 할 일 목록 요청 또는 여러 사용자 작업에 사용합니다.
- 새 요구 사항을 작업으로 기록하고, 작업을 시작하기 전에 in_progress로 표시하며, 완료되는 즉시 completed로 표시합니다.

## 이 도구를 사용하지 않을 때

- 하나의 단순하거나 사소한 작업, 세 개 미만의 사소한 단계로 완료되는 작업 또는 순수한 대화 요청에는 사용하지 않습니다.

## 작업 필드

- subject: 명령형으로 작성한 짧고 실행 가능한 제목입니다.
- description: 작업을 완료하는 데 충분한 세부 정보이며, 팀 모드가 활성화된 경우 팀원에게 필요한 맥락도 포함합니다.
- activeForm: 작업 진행 중에 표시되는 선택적 진행형 텍스트입니다.

모든 새 작업은 owner 없이 pending 상태로 시작합니다. TaskUpdate로 팀원을 지정하고 blocks/blockedBy를 통해 종속성을 설정합니다. 중복 작업을 피하려면 먼저 TaskList를 확인하세요.`,
		`Создать новую задачу в списке задач

Этот инструмент создаёт структурированный список задач для текущего сеанса разработки. Он помогает отслеживать ход работы, организовывать сложные задачи и показывать пользователю общий прогресс.

## Когда использовать этот инструмент

- Для сложной работы с тремя или более различными этапами, существенного планирования, режима планирования, явных запросов на список дел или нескольких пользовательских задач.
- Фиксируйте новые требования как задачи, перед началом работы помечайте задачу как in_progress, а сразу после завершения — как completed.

## Когда не следует использовать этот инструмент

- Не используйте его для одной простой или тривиальной задачи, работы, выполняемой менее чем за три тривиальных шага, и чисто разговорных запросов.

## Поля задачи

- subject: краткий заголовок в повелительной форме, описывающий действие.
- description: достаточно подробностей для выполнения работы; в командном режиме также укажите контекст, необходимый участникам команды.
- activeForm: необязательный текст в форме длительного действия, отображаемый во время выполнения задачи.

Все новые задачи создаются без owner со статусом pending. Используйте TaskUpdate, чтобы назначать участников команды и задавать зависимости через blocks/blockedBy. Сначала проверьте TaskList, чтобы избежать дублирования задач.`)
	add(KeyToolTaskCreateInputSubjectDescription,
		"A brief title for the task.", "任务的简短标题。", "Kurzer Titel der Aufgabe.", "タスクの簡潔なタイトル。", "작업의 간단한 제목입니다.", "Краткое название задачи.")
	add(KeyToolTaskCreateInputDescriptionDescription,
		"What needs to be done.", "需要完成的工作。", "Was erledigt werden soll.", "実行する必要がある作業。", "수행해야 할 작업입니다.", "Что необходимо сделать.")
	add(KeyToolTaskInputActiveFormDescription,
		`Present-continuous text shown while the task is in_progress (for example, "Running tests").`,
		`任务处于 in_progress 时显示的进行时文本（例如“正在运行测试”）。`,
		`Verlaufsform-Text, der angezeigt wird, solange die Aufgabe in_progress ist (zum Beispiel „Tests werden ausgeführt“).`,
		`タスクが in_progress の間に表示する進行形テキスト（例: 「テストを実行中」）。`,
		`작업이 in_progress인 동안 표시되는 진행형 텍스트입니다(예: "테스트 실행 중").`,
		`Текст в форме длительного действия, отображаемый, пока задача имеет статус in_progress (например, «Выполнение тестов»).`)
	add(KeyToolTaskCreateInputMetadataDescription,
		"Arbitrary metadata to attach to the task.", "要附加到任务的任意元数据。", "Beliebige Metadaten für die Aufgabe.", "タスクに付加する任意のメタデータ。", "작업에 첨부할 임의의 메타데이터입니다.", "Произвольные метаданные, добавляемые к задаче.")
	add(KeyToolTaskCreateDiscoveryHint,
		"Create a task in the task list", "在任务列表中创建任务", "Eine Aufgabe in der Aufgabenliste erstellen", "タスクリストにタスクを作成", "작업 목록에 작업 만들기", "Создать задачу в списке задач")
	add(KeyToolTaskListDescription,
		"List all tasks in the task list.", "列出任务列表中的所有任务。", "Listet alle Aufgaben in der Aufgabenliste auf.", "タスクリスト内のすべてのタスクを一覧表示します。", "작업 목록의 모든 작업을 나열합니다.", "Показывает все задачи в списке задач.")
	add(KeyToolTaskListDiscoveryHint,
		"List all tasks", "列出所有任务", "Alle Aufgaben auflisten", "すべてのタスクを一覧表示", "모든 작업 나열", "Показать все задачи")
	add(KeyToolTaskUpdateDescription,
		"Update a task.", "更新任务。", "Aktualisiert eine Aufgabe.", "タスクを更新します。", "작업을 업데이트합니다.", "Обновляет задачу.")
	add(KeyToolTaskUpdateInputTaskIDDescription,
		"ID of the task to update.", "要更新的任务 ID。", "ID der zu aktualisierenden Aufgabe.", "更新するタスクの ID。", "업데이트할 작업의 ID입니다.", "Идентификатор обновляемой задачи.")
	add(KeyToolTaskUpdateInputSubjectDescription,
		"New subject for the task.", "任务的新 subject。", "Neuer subject der Aufgabe.", "タスクの新しい subject。", "작업의 새 subject입니다.", "Новое значение subject задачи.")
	add(KeyToolTaskUpdateInputDescriptionDescription,
		"New description for the task.", "任务的新 description。", "Neue description der Aufgabe.", "タスクの新しい description。", "작업의 새 description입니다.", "Новое значение description задачи.")
	add(KeyToolTaskUpdateInputStatusDescription,
		"New status for the task.", "任务的新 status。", "Neuer status der Aufgabe.", "タスクの新しい status。", "작업의 새 status입니다.", "Новый status задачи.")
	add(KeyToolTaskUpdateInputAddBlocksDescription,
		"Task IDs that this task blocks.", "被此任务阻塞的任务 ID。", "IDs der Aufgaben, die von dieser Aufgabe blockiert werden.", "このタスクがブロックするタスクの ID。", "이 작업이 차단하는 작업 ID입니다.", "Идентификаторы задач, которые блокирует эта задача.")
	add(KeyToolTaskUpdateInputAddBlockedByDescription,
		"Task IDs that block this task.", "阻塞此任务的任务 ID。", "IDs der Aufgaben, die diese Aufgabe blockieren.", "このタスクをブロックするタスクの ID。", "이 작업을 차단하는 작업 ID입니다.", "Идентификаторы задач, блокирующих эту задачу.")
	add(KeyToolTaskUpdateInputOwnerDescription,
		"New owner for the task.", "任务的新 owner。", "Neuer owner der Aufgabe.", "タスクの新しい owner。", "작업의 새 owner입니다.", "Новый owner задачи.")
	add(KeyToolTaskUpdateInputMetadataDescription,
		"Metadata keys to merge into the task. Set a key to null to delete it.",
		"要合并到任务中的元数据键；将某个键设为 null 可删除该键。",
		"Metadatenschlüssel, die in die Aufgabe übernommen werden. Zum Löschen einen Schlüssel auf null setzen.",
		"タスクにマージするメタデータキー。削除するにはキーを null に設定します。",
		"작업에 병합할 메타데이터 키입니다. 키를 삭제하려면 null로 설정하세요.",
		"Ключи метаданных, добавляемые в задачу. Чтобы удалить ключ, задайте ему значение null.")
	add(KeyToolTaskUpdateDiscoveryHint,
		"Update a task", "更新任务", "Eine Aufgabe aktualisieren", "タスクを更新", "작업 업데이트", "Обновить задачу")
	add(KeyToolTaskGetDescription,
		"Get a task by ID from the task list.", "按 ID 从任务列表中获取任务。", "Ruft eine Aufgabe anhand ihrer ID aus der Aufgabenliste ab.", "ID を指定してタスクリストからタスクを取得します。", "ID로 작업 목록에서 작업을 가져옵니다.", "Получает задачу из списка по идентификатору.")
	add(KeyToolTaskGetInputTaskIDDescription,
		"ID of the task to retrieve.", "要获取的任务 ID。", "ID der abzurufenden Aufgabe.", "取得するタスクの ID。", "가져올 작업의 ID입니다.", "Идентификатор задачи, которую нужно получить.")
	add(KeyToolTaskGetDiscoveryHint,
		"Retrieve a task by ID", "按 ID 获取任务", "Eine Aufgabe anhand ihrer ID abrufen", "ID を指定してタスクを取得", "ID로 작업 가져오기", "Получить задачу по идентификатору")

	add(KeyToolTaskStopDescription,
		"Stop a running background task by ID.",
		"按 ID 停止正在运行的后台任务。",
		"Beendet eine laufende Hintergrundaufgabe anhand ihrer ID.",
		"ID を指定して実行中のバックグラウンドタスクを停止します。",
		"ID로 실행 중인 백그라운드 작업을 중지합니다.",
		"Останавливает выполняющуюся фоновую задачу по идентификатору.")
	add(KeyToolTaskStopInputTaskIDDescription,
		"ID of the background task to stop.",
		"要停止的后台任务 ID。",
		"ID der zu beendenden Hintergrundaufgabe.",
		"停止するバックグラウンドタスクの ID。",
		"중지할 백그라운드 작업의 ID입니다.",
		"Идентификатор фоновой задачи, которую нужно остановить.")
	add(KeyToolTaskOutputDescription,
		"Retrieve output from a running or completed background task.",
		"获取正在运行或已完成的后台任务输出。",
		"Ruft die Ausgabe einer laufenden oder abgeschlossenen Hintergrundaufgabe ab.",
		"実行中または完了したバックグラウンドタスクの出力を取得します。",
		"실행 중이거나 완료된 백그라운드 작업의 출력을 가져옵니다.",
		"Получает вывод выполняющейся или завершённой фоновой задачи.")
	add(KeyToolTaskOutputInputTaskIDDescription,
		"ID of the background task whose output to retrieve.",
		"要获取输出的后台任务 ID。",
		"ID der Hintergrundaufgabe, deren Ausgabe abgerufen wird.",
		"出力を取得するバックグラウンドタスクの ID。",
		"출력을 가져올 백그라운드 작업의 ID입니다.",
		"Идентификатор фоновой задачи, вывод которой нужно получить.")
	add(KeyToolTaskOutputInputBlockDescription,
		"Whether to wait for completion before returning.",
		"返回前是否等待任务完成。",
		"Ob vor der Rückgabe auf den Abschluss gewartet werden soll.",
		"結果を返す前にタスクの完了を待つかどうか。",
		"결과를 반환하기 전에 작업 완료를 기다릴지 여부입니다.",
		"Следует ли дождаться завершения задачи перед возвратом результата.")
	add(KeyToolTaskOutputInputTimeoutDescription,
		"Maximum wait time in milliseconds.",
		"最长等待时间（毫秒）。",
		"Maximale Wartezeit in Millisekunden.",
		"最大待機時間（ミリ秒）。",
		"최대 대기 시간(밀리초)입니다.",
		"Максимальное время ожидания в миллисекундах.")
}
