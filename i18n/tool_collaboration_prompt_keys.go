package i18n

const (
	KeyToolTeamCreateDescription                  Key = "tool.team_create.description"
	KeyToolTeamDeleteDescription                  Key = "tool.team_delete.description"
	KeyToolSendMessageDescription                 Key = "tool.send_message.description"
	KeyToolTeamCreateTeamNameDescription          Key = "tool.team_create.input.team_name.description"
	KeyToolTeamCreatePurposeDescription           Key = "tool.team_create.input.description.description"
	KeyToolTeamCreateAgentTypeDescription         Key = "tool.team_create.input.agent_type.description"
	KeyToolSendMessageToDescription               Key = "tool.send_message.input.to.description"
	KeyToolSendMessageSummaryDescription          Key = "tool.send_message.input.summary.description"
	KeyToolSendMessageMessageDescription          Key = "tool.send_message.input.message.description"
	KeyToolSendMessagePlainTextDescription        Key = "tool.send_message.input.message.plain_text.description"
	KeyToolCollaborationRuntimeIdentityIncomplete Key = "tool.collaboration.runtime_identity.incomplete"
	KeyToolCollaborationManagerRequired           Key = "tool.collaboration.manager.required"
	KeyToolCollaborationSpawnReservationMissing   Key = "tool.collaboration.spawn.reservation_missing"
)

var toolCollaborationPromptKeys = [...]Key{
	KeyToolTeamCreateDescription,
	KeyToolTeamDeleteDescription,
	KeyToolSendMessageDescription,
	KeyToolTeamCreateTeamNameDescription,
	KeyToolTeamCreatePurposeDescription,
	KeyToolTeamCreateAgentTypeDescription,
	KeyToolSendMessageToDescription,
	KeyToolSendMessageSummaryDescription,
	KeyToolSendMessageMessageDescription,
	KeyToolSendMessagePlainTextDescription,
	KeyToolCollaborationRuntimeIdentityIncomplete,
	KeyToolCollaborationManagerRequired,
	KeyToolCollaborationSpawnReservationMissing,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}

	add(KeyToolTeamCreateDescription,
		"Create a team of agents for parallel work",
		"创建 Agent 团队以并行处理工作",
		"Ein Team aus Agents für parallele Arbeit erstellen",
		"並列作業のための Agent チームを作成します",
		"병렬 작업을 위한 Agent 팀을 만듭니다",
		"Создать команду агентов для параллельной работы")
	add(KeyToolTeamDeleteDescription,
		"Delete a team and its agents",
		"删除团队及其 Agent",
		"Ein Team und seine Agents löschen",
		"チームとその Agent を削除します",
		"팀과 소속 Agent를 삭제합니다",
		"Удалить команду и её агентов")
	add(KeyToolSendMessageDescription,
		"Send a message to another agent",
		"向另一个 Agent 发送消息",
		"Eine Nachricht an einen anderen Agent senden",
		"別の Agent にメッセージを送信します",
		"다른 Agent에게 메시지를 보냅니다",
		"Отправить сообщение другому агенту")
	add(KeyToolTeamCreateTeamNameDescription,
		"Name for the new team to create.",
		"要创建的新团队的名称。",
		"Name des neu zu erstellenden Teams.",
		"作成する新しいチームの名前です。",
		"만들 새 팀의 이름입니다.",
		"Название создаваемой команды.")
	add(KeyToolTeamCreatePurposeDescription,
		"Team description/purpose.",
		"团队的说明或用途。",
		"Beschreibung oder Zweck des Teams.",
		"チームの説明または目的です。",
		"팀의 설명 또는 목적입니다.",
		"Описание или назначение команды.")
	add(KeyToolTeamCreateAgentTypeDescription,
		`Type/role of the team lead (e.g., "researcher", "test-runner"). Used for team file and inter-agent coordination.`,
		`团队负责人的类型或角色（例如 "researcher"、"test-runner"）。用于团队文件和 Agent 间协调。`,
		`Typ oder Rolle der Teamleitung (z. B. "researcher", "test-runner"). Wird für die Teamdatei und die Koordination zwischen Agents verwendet.`,
		`チームリーダーのタイプまたは役割（例: "researcher"、"test-runner"）。チームファイルと Agent 間の連携に使用します。`,
		`팀 리드의 유형 또는 역할(예: "researcher", "test-runner")입니다. 팀 파일과 Agent 간 조정에 사용됩니다.`,
		`Тип или роль руководителя команды (например, "researcher", "test-runner"). Используется для файла команды и координации между агентами.`)
	add(KeyToolSendMessageToDescription,
		`Recipient: teammate name, "*" for broadcast, or "uds:<socket-path>" for a local peer`,
		`收件人：teammate 名称，广播使用 "*"，本地对等端使用 "uds:<socket-path>"`,
		`Empfänger: Name des Teammate, "*" für Broadcast oder "uds:<socket-path>" für einen lokalen Peer`,
		`送信先: teammate 名、ブロードキャストの場合は "*"、ローカル peer の場合は "uds:<socket-path>"`,
		`받는 사람: teammate 이름, 브로드캐스트에는 "*", 로컬 peer에는 "uds:<socket-path>"`,
		`Получатель: имя teammate, "*" для рассылки или "uds:<socket-path>" для локального узла`)
	add(KeyToolSendMessageSummaryDescription,
		"A 5-10 word summary shown as a preview in the UI (required when message is a string)",
		"在 UI 中作为预览显示的 5–10 个词的摘要（message 为字符串时必填）",
		"Eine Zusammenfassung mit 5–10 Wörtern, die als Vorschau in der UI angezeigt wird (erforderlich, wenn message eine Zeichenfolge ist)",
		"UI にプレビュー表示する5～10語の要約（message が文字列の場合は必須）",
		"UI에 미리보기로 표시되는 5~10단어 요약(message가 문자열인 경우 필수)",
		"Краткое описание из 5–10 слов для предварительного просмотра в UI (обязательно, если message — строка)")
	add(KeyToolSendMessageMessageDescription,
		"Plain text message content or a structured swarm control message",
		"纯文本消息内容或结构化的 swarm 控制消息",
		"Nachrichteninhalt als Klartext oder strukturierte swarm-Steuernachricht",
		"プレーンテキストのメッセージ内容または構造化された swarm 制御メッセージ",
		"일반 텍스트 메시지 내용 또는 구조화된 swarm 제어 메시지",
		"Текстовое содержимое сообщения или структурированное управляющее сообщение swarm")
	add(KeyToolSendMessagePlainTextDescription,
		"Plain text message content",
		"纯文本消息内容",
		"Nachrichteninhalt als Klartext",
		"プレーンテキストのメッセージ内容",
		"일반 텍스트 메시지 내용",
		"Содержимое сообщения в виде обычного текста")
	add(KeyToolCollaborationRuntimeIdentityIncomplete,
		"active runtime identity is incomplete",
		"当前运行时标识不完整",
		"Die Identität der aktiven Laufzeit ist unvollständig",
		"アクティブなランタイム識別情報が不完全です",
		"활성 런타임 식별 정보가 완전하지 않습니다",
		"Идентификатор активной среды выполнения неполон")
	add(KeyToolCollaborationManagerRequired,
		"team manager is required",
		"必须提供团队管理器",
		"Ein Team-Manager ist erforderlich",
		"team manager が必要です",
		"team manager가 필요합니다",
		"Требуется менеджер команды")
	add(KeyToolCollaborationSpawnReservationMissing,
		"Teammate spawn reservation %q is missing.",
		"找不到 teammate 生成预留 %q。",
		"Die Reservierung %q zum Starten des Teammate fehlt.",
		"teammate 起動予約 %q が見つかりません。",
		"teammate 생성 예약 %q이(가) 없습니다.",
		"Резервирование %q для запуска teammate отсутствует.")
}
