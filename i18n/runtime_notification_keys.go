package i18n

const KeyToolNotificationPersistenceFailed Key = "tool.notification.persistence_failed"

func init() {
	semanticTranslations[KeyToolNotificationPersistenceFailed] = map[Language]string{
		LangEN: "Could not save background notification state.",
		LangZH: "无法保存后台通知状态。",
		LangDE: "Der Status der Hintergrundbenachrichtigung konnte nicht gespeichert werden.",
		LangJA: "バックグラウンド通知の状態を保存できませんでした。",
		LangKO: "백그라운드 알림 상태를 저장하지 못했습니다.",
		LangRU: "Не удалось сохранить состояние фонового уведомления.",
	}
}
