package i18n

import "testing"

func TestToolPresentationActionCatalogCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyToolActionRunCommand, KeyToolActionReadFile, KeyToolActionCreateFile, KeyToolActionUpdateFile,
		KeyToolActionEditNotebook, KeyToolActionFindFiles, KeyToolActionSearchText, KeyToolActionInspectCode,
		KeyToolActionFindTools, KeyToolActionFetchWeb, KeyToolActionSearchWeb, KeyToolActionUseMCPTool,
		KeyToolActionListMCPResources, KeyToolActionReadMCPResource, KeyToolActionRunAgent,
		KeyToolActionCreateTask, KeyToolActionListTasks, KeyToolActionUpdateTask, KeyToolActionGetTask,
		KeyToolActionStopTask, KeyToolActionReadTaskOutput, KeyToolActionGetGoal,
		KeyToolActionCreateGoal, KeyToolActionUpdateGoal, KeyToolActionEnterPlanMode, KeyToolActionExitPlanMode,
		KeyToolActionAskUser, KeyToolActionSendUserMessage, KeyToolActionSendMessage, KeyToolActionCreateTeam,
		KeyToolActionDeleteTeam, KeyToolActionCreateSchedule, KeyToolActionDeleteSchedule, KeyToolActionListSchedules,
		KeyToolActionEnterWorktree, KeyToolActionExitWorktree, KeyToolActionReadConfiguration, KeyToolActionConfigure, KeyToolActionLoadSkill,
		KeyToolActionRemoteRequest, KeyToolEmptyMatches, KeyToolEmptyFiles, KeyToolEmptyTools, KeyToolEmptySources,
		KeyToolEmptyResources, KeyToolEmptyTasks, KeyToolEmptySchedules,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("missing translation for %s in %s", key, lang.Code())
			}
		}
	}
}
