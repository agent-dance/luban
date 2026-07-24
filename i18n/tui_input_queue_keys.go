package i18n

const (
	KeyTUIInputQueuedStatus     Key = "tui.input.queue.status"
	KeyTUIInputQueuedAsGuidance Key = "tui.input.queue.guidance"
	KeyTUIExitConfirm           Key = "tui.exit.confirm"
)

func init() {
	semanticTranslations[KeyTUIInputQueuedStatus] = repl(
		"%d queued · Esc to steer",
		"%d 条消息排队中 · 按 Esc 转为引导",
		"%d eingereiht · Esc zum Steuern",
		"%d 件待機中 · Esc で指示に変更",
		"%d개 대기 중 · Esc로 지시 전환",
		"в очереди: %d · Esc — направить работу",
	)
	semanticTranslations[KeyTUIInputQueuedAsGuidance] = repl(
		"Queued message will steer the next work",
		"已将排队消息转为引导，将用于指导接下来的工作",
		"Die eingereihte Nachricht steuert die nächste Arbeit",
		"待機中のメッセージで次の作業を指示します",
		"대기 중인 메시지로 다음 작업을 안내합니다",
		"Сообщение из очереди направит следующую работу",
	)
	semanticTranslations[KeyTUIExitConfirm] = repl(
		"Press Ctrl+C again to exit",
		"再次按 Ctrl+C 退出",
		"Zum Beenden erneut Ctrl+C drücken",
		"終了するにはもう一度 Ctrl+C を押してください",
		"종료하려면 Ctrl+C를 다시 누르세요",
		"Для выхода снова нажмите Ctrl+C",
	)
}
