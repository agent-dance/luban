package i18n

const (
	KeyToolPlanModeStateMissing        Key = "tool.plan_mode.error.state_missing"
	KeyToolPlanModeAgentContext        Key = "tool.plan_mode.error.agent_context"
	KeyToolPlanModeEnterPermission     Key = "tool.plan_mode.error.enter_permission"
	KeyToolPlanModePersistState        Key = "tool.plan_mode.error.persist_state"
	KeyToolPlanModeCreateDirectory     Key = "tool.plan_mode.error.create_directory"
	KeyToolPlanModeCreatePlans         Key = "tool.plan_mode.error.create_plans"
	KeyToolPlanModeCreatePlanDirectory Key = "tool.plan_mode.error.create_plan_directory"
)

var toolPlanModeErrorKeys = [...]Key{
	KeyToolPlanModeStateMissing,
	KeyToolPlanModeAgentContext,
	KeyToolPlanModeEnterPermission,
	KeyToolPlanModePersistState,
	KeyToolPlanModeCreateDirectory,
	KeyToolPlanModeCreatePlans,
	KeyToolPlanModeCreatePlanDirectory,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
	}
	add(KeyToolPlanModeStateMissing,
		"EnterPlanMode requires plan state", "EnterPlanMode 需要 Plan 状态", "EnterPlanMode benötigt einen Plan-Status", "EnterPlanMode には Plan の状態が必要です", "EnterPlanMode에는 Plan 상태가 필요합니다", "Для EnterPlanMode требуется состояние Plan")
	add(KeyToolPlanModeAgentContext,
		"EnterPlanMode tool cannot be used in agent contexts", "EnterPlanMode Tool 不能在 Agent 上下文中使用", "Das Tool EnterPlanMode kann nicht in Agent-Kontexten verwendet werden", "EnterPlanMode Tool は Agent コンテキストでは使用できません", "EnterPlanMode Tool은 Agent 컨텍스트에서 사용할 수 없습니다", "Tool EnterPlanMode нельзя использовать в контексте Agent")
	add(KeyToolPlanModeEnterPermission,
		"enter plan permission mode: %v", "进入 Plan 权限模式失败：%v", "Plan-Berechtigungsmodus konnte nicht aktiviert werden: %v", "Plan 権限モードに切り替えられませんでした: %v", "Plan 권한 모드로 전환하지 못했습니다: %v", "Не удалось включить режим разрешений Plan: %v")
	add(KeyToolPlanModePersistState,
		"persist plan mode state: %v", "保存 Plan 模式状态失败：%v", "Status des Plan-Modus konnte nicht gespeichert werden: %v", "Plan モードの状態を保存できませんでした: %v", "Plan 모드 상태를 저장하지 못했습니다: %v", "Не удалось сохранить состояние режима Plan: %v")
	add(KeyToolPlanModeCreateDirectory,
		"mkdir %s: %v", "创建目录 %s 失败：%v", "Verzeichnis %s konnte nicht erstellt werden: %v", "ディレクトリ %s を作成できませんでした: %v", "디렉터리 %s을(를) 만들지 못했습니다: %v", "Не удалось создать каталог %s: %v")
	add(KeyToolPlanModeCreatePlans,
		"mkdir plans: %v", "创建 plans 目录失败：%v", "Plans-Verzeichnis konnte nicht erstellt werden: %v", "plans ディレクトリを作成できませんでした: %v", "plans 디렉터리를 만들지 못했습니다: %v", "Не удалось создать каталог plans: %v")
	add(KeyToolPlanModeCreatePlanDirectory,
		"mkdir plan dir: %v", "创建 Plan 目录失败：%v", "Plan-Verzeichnis konnte nicht erstellt werden: %v", "Plan ディレクトリを作成できませんでした: %v", "Plan 디렉터리를 만들지 못했습니다: %v", "Не удалось создать каталог Plan: %v")
}
