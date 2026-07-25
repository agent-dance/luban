package i18n

// Model-facing descriptions and schema guidance for interactive tools.
const (
	KeyToolInteractionAskUserDescription   Key = "tool.interaction.ask_user.description"
	KeyToolInteractionAskUserQuestion      Key = "tool.interaction.ask_user.schema.question"
	KeyToolInteractionAskUserHeader        Key = "tool.interaction.ask_user.schema.header"
	KeyToolInteractionEnterPlanDescription Key = "tool.interaction.enter_plan.description"
	KeyToolInteractionExitPlanDescription  Key = "tool.interaction.exit_plan.description"
	KeyToolInteractionExitPlanPermissions  Key = "tool.interaction.exit_plan.schema.permissions"
)

var toolInteractionPromptKeys = [...]Key{
	KeyToolInteractionAskUserDescription,
	KeyToolInteractionAskUserQuestion,
	KeyToolInteractionAskUserHeader,
	KeyToolInteractionEnterPlanDescription,
	KeyToolInteractionExitPlanDescription,
	KeyToolInteractionExitPlanPermissions,
}

func init() {
	semanticTranslations[KeyToolInteractionAskUserDescription] = map[Language]string{
		LangEN: "Ask the user one or more questions with structured choices.",
		LangZH: "通过结构化选项向用户提出一个或多个问题。",
		LangDE: "Stelle dem Benutzer eine oder mehrere Fragen mit strukturierten Auswahlmöglichkeiten.",
		LangJA: "構造化された選択肢を使って、ユーザーに1つ以上の質問をします。",
		LangKO: "구조화된 선택지로 사용자에게 하나 이상의 질문을 합니다.",
		LangRU: "Задать пользователю один или несколько вопросов со структурированными вариантами ответа.",
	}
	semanticTranslations[KeyToolInteractionAskUserQuestion] = map[Language]string{
		LangEN: "The question to ask.",
		LangZH: "要询问的问题。",
		LangDE: "Die zu stellende Frage.",
		LangJA: "ユーザーに尋ねる質問。",
		LangKO: "사용자에게 물을 질문입니다.",
		LangRU: "Вопрос, который нужно задать.",
	}
	semanticTranslations[KeyToolInteractionAskUserHeader] = map[Language]string{
		LangEN: "A short label of at most 12 characters.",
		LangZH: "不超过 12 个字符的简短标签。",
		LangDE: "Eine kurze Bezeichnung mit höchstens 12 Zeichen.",
		LangJA: "12文字以内の短いラベル。",
		LangKO: "최대 12자의 짧은 레이블입니다.",
		LangRU: "Короткая метка длиной не более 12 символов.",
	}
	semanticTranslations[KeyToolInteractionEnterPlanDescription] = map[Language]string{
		LangEN: "Enter read-only plan mode to explore the project and design an implementation approach for user approval.",
		LangZH: "进入只读 plan 模式，探索项目并设计实现方案以供用户审批。",
		LangDE: "Wechsle in den schreibgeschützten plan-Modus, um das Projekt zu untersuchen und einen Umsetzungsansatz zur Genehmigung zu entwerfen.",
		LangJA: "読み取り専用の plan モードに入り、プロジェクトを調査して、承認を得るための実装方針を設計します。",
		LangKO: "읽기 전용 plan 모드로 전환하여 프로젝트를 살펴보고 승인을 받을 구현 방안을 설계합니다.",
		LangRU: "Перейти в режим plan только для чтения, изучить проект и подготовить подход к реализации для утверждения пользователем.",
	}
	semanticTranslations[KeyToolInteractionExitPlanDescription] = map[Language]string{
		LangEN: "Submit the plan for approval and leave plan mode before implementation.",
		LangZH: "提交计划以供审批，并在实施前退出 plan 模式。",
		LangDE: "Reiche den Plan zur Genehmigung ein und verlasse vor der Umsetzung den plan-Modus.",
		LangJA: "計画を承認用に提出し、実装前に plan モードを終了します。",
		LangKO: "계획을 승인받기 위해 제출하고 구현 전에 plan 모드를 종료합니다.",
		LangRU: "Отправить план на утверждение и выйти из режима plan перед реализацией.",
	}
	semanticTranslations[KeyToolInteractionExitPlanPermissions] = map[Language]string{
		LangEN: "Permission categories needed to implement the plan; describe actions, not exact commands.",
		LangZH: "实施计划所需的权限类别；描述操作类型，而不是具体命令。",
		LangDE: "Für die Umsetzung benötigte Berechtigungskategorien; beschreibe Aktionen statt exakter Befehle.",
		LangJA: "計画の実装に必要な権限カテゴリ。具体的なコマンドではなく操作を記述します。",
		LangKO: "계획 구현에 필요한 권한 범주입니다. 정확한 명령이 아니라 작업을 설명합니다.",
		LangRU: "Категории разрешений, необходимые для реализации плана; описывайте действия, а не точные команды.",
	}
}
