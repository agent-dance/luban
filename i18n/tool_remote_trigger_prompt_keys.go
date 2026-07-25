package i18n

// Semantic copy exposed through the RemoteTrigger tool description and input
// schema. CCR and field names are protocol identifiers and remain unchanged.
const (
	KeyToolRemoteTriggerDescription    Key = "tool.remote_trigger.description"
	KeyToolRemoteTriggerInputAction    Key = "tool.remote_trigger.input.action.description"
	KeyToolRemoteTriggerInputTriggerID Key = "tool.remote_trigger.input.trigger_id.description"
	KeyToolRemoteTriggerInputBody      Key = "tool.remote_trigger.input.body.description"
)

var toolRemoteTriggerPromptKeys = [...]Key{
	KeyToolRemoteTriggerDescription,
	KeyToolRemoteTriggerInputAction,
	KeyToolRemoteTriggerInputTriggerID,
	KeyToolRemoteTriggerInputBody,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolRemoteTriggerDescription,
		"Manage scheduled remote-agent triggers through the CCR API. Authentication stays inside the LUBAN Code process and is never exposed to shell commands.",
		"通过 CCR API 管理远程 Agent 的计划触发器。认证信息始终保留在 LUBAN Code 进程内，绝不会暴露给 shell 命令。",
		"Verwaltet geplante Trigger für Remote-Agents über die CCR API. Authentifizierungsdaten verbleiben im LUBAN Code-Prozess und werden niemals Shell-Befehlen offengelegt.",
		"CCR API を通じてリモート Agent のスケジュール済み trigger を管理します。認証情報は LUBAN Code プロセス内に保持され、shell command には公開されません。",
		"CCR API를 통해 원격 Agent의 예약 trigger를 관리합니다. 인증 정보는 LUBAN Code 프로세스 내부에만 유지되며 shell command에 노출되지 않습니다.",
		"Управляет запланированными trigger удалённых Agent через CCR API. Данные аутентификации остаются внутри процесса LUBAN Code и не передаются shell command.")
	add(KeyToolRemoteTriggerInputAction,
		"Action to perform on remote triggers",
		"要对远程触发器执行的操作",
		"Aktion für Remote-Trigger",
		"リモート trigger に対して実行する操作",
		"원격 trigger에 수행할 작업",
		"Действие с удалёнными trigger")
	add(KeyToolRemoteTriggerInputTriggerID,
		"Trigger identifier required by get, update, and run",
		"get、update 和 run 所需的触发器标识符",
		"Trigger-Kennung, die für get, update und run erforderlich ist",
		"get、update、run に必要な trigger 識別子",
		"get, update, run에 필요한 trigger 식별자",
		"Идентификатор trigger, необходимый для get, update и run")
	add(KeyToolRemoteTriggerInputBody,
		"JSON body required by create and update",
		"create 和 update 所需的 JSON body",
		"Für create und update erforderlicher JSON-Body",
		"create と update に必要な JSON body",
		"create와 update에 필요한 JSON body",
		"JSON body, необходимое для create и update")
}
