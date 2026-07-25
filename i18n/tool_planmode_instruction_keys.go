package i18n

// Semantic instructions returned to the model after EnterPlanMode succeeds.
// Tool names and the phase protocol term remain stable identifiers across
// languages; the localized entry message is supplied as the sole argument.
const (
	KeyToolPlanModeInstructions Key = "tool.plan_mode.instructions.standard"
)

var toolPlanModeInstructionKeys = []Key{
	KeyToolPlanModeInstructions,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(
		KeyToolPlanModeInstructions,
		"%s\n\nIn plan mode, you should:\n1. Thoroughly explore the codebase to understand existing patterns\n2. Identify similar features and architectural approaches\n3. Consider multiple approaches and their trade-offs\n4. Use AskUserQuestion if you need to clarify the approach\n5. Design a concrete implementation strategy\n6. When ready, use ExitPlanMode to present your plan for approval\n\nRemember: DO NOT write or edit any files yet. This is a read-only exploration and planning phase.",
		"%s\n\n在 plan mode 中，你应该：\n1. 深入探索代码库，了解现有模式\n2. 找出类似功能及其架构方案\n3. 考虑多种方案及其权衡\n4. 如果需要澄清方案，请使用 AskUserQuestion\n5. 设计具体的实施策略\n6. 准备好后，使用 ExitPlanMode 提交计划以供批准\n\n请记住：现在不要写入或编辑任何文件。当前 phase 仅用于只读探索与规划。",
		"%s\n\nIm plan mode solltest du:\n1. Die Codebasis gründlich erkunden, um bestehende Muster zu verstehen\n2. Ähnliche Funktionen und Architekturansätze ermitteln\n3. Mehrere Ansätze und ihre Vor- und Nachteile abwägen\n4. AskUserQuestion verwenden, wenn du den Ansatz genauer klären musst\n5. Eine konkrete Umsetzungsstrategie entwerfen\n6. Wenn du bereit bist, mit ExitPlanMode deinen Plan zur Genehmigung vorlegen\n\nDenk daran: Schreibe oder bearbeite noch keine Dateien. Diese phase dient ausschließlich der schreibgeschützten Erkundung und Planung.",
		"%s\n\nplan mode では、次のことを行ってください：\n1. コードベースを詳しく調査し、既存のパターンを理解する\n2. 類似機能とアーキテクチャ上のアプローチを特定する\n3. 複数のアプローチと、そのトレードオフを検討する\n4. アプローチを明確にする必要がある場合は AskUserQuestion を使用する\n5. 具体的な実装戦略を設計する\n6. 準備ができたら ExitPlanMode を使用し、承認を得るために計画を提示する\n\n注意：まだファイルへの書き込みや編集は行わないでください。この phase は読み取り専用の調査と計画のためのものです。",
		"%s\n\nplan mode에서는 다음을 수행해야 합니다:\n1. 코드베이스를 충분히 탐색하여 기존 패턴을 파악합니다\n2. 유사한 기능과 아키텍처 접근 방식을 찾습니다\n3. 여러 접근 방식과 각각의 장단점을 검토합니다\n4. 접근 방식을 명확히 해야 한다면 AskUserQuestion을 사용합니다\n5. 구체적인 구현 전략을 설계합니다\n6. 준비가 되면 ExitPlanMode로 계획을 제시하고 승인을 요청합니다\n\n기억하세요: 아직 어떤 파일도 작성하거나 편집하지 마세요. 이 phase는 읽기 전용 탐색과 계획을 위한 것입니다.",
		"%s\n\nВ plan mode необходимо:\n1. Тщательно изучить кодовую базу и понять существующие шаблоны\n2. Найти похожие функции и архитектурные подходы\n3. Рассмотреть несколько подходов и их компромиссы\n4. Использовать AskUserQuestion, если подход нужно уточнить\n5. Разработать конкретную стратегию реализации\n6. Когда всё будет готово, представить план на утверждение с помощью ExitPlanMode\n\nПомните: пока не записывайте и не редактируйте файлы. Эта phase предназначена только для исследования и планирования в режиме чтения.",
	)
}
