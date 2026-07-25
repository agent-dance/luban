package i18n

// Model-facing descriptions and schema guidance for SendUserMessage.
const (
	KeyToolSendUserMessageDescription                 Key = "tool.send_user_message.description"
	KeyToolSendUserMessageInputMessageDescription     Key = "tool.send_user_message.input.message.description"
	KeyToolSendUserMessageInputAttachmentsDescription Key = "tool.send_user_message.input.attachments.description"
	KeyToolSendUserMessageInputStatusDescription      Key = "tool.send_user_message.input.status.description"
	KeyToolSendUserMessageDiscoveryHint               Key = "tool.send_user_message.discovery_hint"
)

var toolSendUserMessagePromptKeys = [...]Key{
	KeyToolSendUserMessageDescription,
	KeyToolSendUserMessageInputMessageDescription,
	KeyToolSendUserMessageInputAttachmentsDescription,
	KeyToolSendUserMessageInputStatusDescription,
	KeyToolSendUserMessageDiscoveryHint,
}

func init() {
	addToolSendUserMessagePrompt(KeyToolSendUserMessageDescription,
		"Send a message to the user",
		"向用户发送消息",
		"Eine Nachricht an den Benutzer senden",
		"ユーザーにメッセージを送信します",
		"사용자에게 메시지를 보냅니다",
		"Отправить сообщение пользователю")
	addToolSendUserMessagePrompt(KeyToolSendUserMessageInputMessageDescription,
		"The message for the user. Supports Markdown formatting.",
		"发送给用户的消息，支持 Markdown 格式。",
		"Die Nachricht für den Benutzer. Markdown-Formatierung wird unterstützt.",
		"ユーザー向けのメッセージです。Markdown 形式を使用できます。",
		"사용자에게 보낼 메시지입니다. Markdown 형식을 지원합니다.",
		"Сообщение для пользователя. Поддерживается форматирование Markdown.")
	addToolSendUserMessagePrompt(KeyToolSendUserMessageInputAttachmentsDescription,
		"Optional file paths to attach, either absolute or relative to the working directory.",
		"要附加的可选文件路径，可以是绝对路径或相对于工作目录的路径。",
		"Optionale anzuhängende Dateipfade, entweder absolut oder relativ zum Arbeitsverzeichnis.",
		"添付する任意のファイルパスです。絶対パスまたは作業ディレクトリからの相対パスを指定できます。",
		"첨부할 선택적 파일 경로입니다. 절대 경로나 작업 디렉터리 기준 상대 경로를 사용할 수 있습니다.",
		"Необязательные пути к прикрепляемым файлам: абсолютные или относительно рабочего каталога.")
	addToolSendUserMessagePrompt(KeyToolSendUserMessageInputStatusDescription,
		"Use proactive for unsolicited updates and normal for replies.",
		"主动更新使用 proactive，回复使用 normal。",
		"Verwende proactive für unaufgeforderte Aktualisierungen und normal für Antworten.",
		"自発的な更新には proactive、返信には normal を使用します。",
		"요청하지 않은 업데이트에는 proactive를, 답변에는 normal을 사용합니다.",
		"Используйте proactive для инициативных обновлений, а normal — для ответов.")
	addToolSendUserMessagePrompt(KeyToolSendUserMessageDiscoveryHint,
		"send a message to the user — the primary visible output channel",
		"向用户发送消息——主要的可见输出通道",
		"eine Nachricht an den Benutzer senden — der primäre sichtbare Ausgabekanal",
		"ユーザーにメッセージを送信 — 主となる可視出力チャンネル",
		"사용자에게 메시지 보내기 — 기본 가시 출력 채널",
		"отправить сообщение пользователю — основной видимый канал вывода")
}

func addToolSendUserMessagePrompt(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
