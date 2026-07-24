package i18n

const KeyREPLNotificationSessionChanged Key = "repl.notification.session_changed"

func init() {
	semanticTranslations[KeyREPLNotificationSessionChanged] = map[Language]string{
		LangEN: "The notification target is no longer the visible session.",
		LangZH: "通知目标已不再是当前可见会话。",
		LangDE: "Das Benachrichtigungsziel ist nicht mehr die sichtbare Sitzung.",
		LangJA: "通知先は現在表示中のセッションではありません。",
		LangKO: "알림 대상이 더 이상 현재 표시 중인 세션이 아닙니다.",
		LangRU: "Целевой сеанс уведомления больше не является видимым.",
	}
}
