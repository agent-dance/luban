package i18n

// Semantic copy for the structured Run tool. Step IDs, argv values, paths,
// protocol status/effect/resource values, exit codes, durations, and raw
// process output remain format arguments or structured protocol fields.
const (
	KeyToolRunDescription               Key = "tool.run.description"
	KeyToolRunSchemaSteps               Key = "tool.run.schema.steps"
	KeyToolRunSchemaStepID              Key = "tool.run.schema.step_id"
	KeyToolRunSchemaArgv                Key = "tool.run.schema.argv"
	KeyToolRunSchemaShellScript         Key = "tool.run.schema.shell_script"
	KeyToolRunSchemaCWD                 Key = "tool.run.schema.cwd"
	KeyToolRunSchemaTimeout             Key = "tool.run.schema.timeout"
	KeyToolRunSchemaDependsOn           Key = "tool.run.schema.depends_on"
	KeyToolRunSchemaFailFast            Key = "tool.run.schema.fail_fast"
	KeyToolRunSchemaHead                Key = "tool.run.schema.head"
	KeyToolRunSchemaTail                Key = "tool.run.schema.tail"
	KeyToolRunSchemaMaxChars            Key = "tool.run.schema.max_chars"
	KeyToolRunSchemaRequiresPatchCommit Key = "tool.run.schema.requires_patch_commit"

	KeyToolRunInvalidInput        Key = "tool.run.error.invalid_input"
	KeyToolRunStepsRequired       Key = "tool.run.error.steps_required"
	KeyToolRunTooManySteps        Key = "tool.run.error.too_many_steps"
	KeyToolRunStepIDRequired      Key = "tool.run.error.step_id_required"
	KeyToolRunStepIDInvalid       Key = "tool.run.error.step_id_invalid"
	KeyToolRunStepIDDuplicate     Key = "tool.run.error.step_id_duplicate"
	KeyToolRunCommandChoice       Key = "tool.run.error.command_choice"
	KeyToolRunArgumentInvalid     Key = "tool.run.error.argument_invalid"
	KeyToolRunCWDInvalid          Key = "tool.run.error.cwd_invalid"
	KeyToolRunCWDNotDirectory     Key = "tool.run.error.cwd_not_directory"
	KeyToolRunTimeoutInvalid      Key = "tool.run.error.timeout_invalid"
	KeyToolRunDependencyUnknown   Key = "tool.run.error.dependency_unknown"
	KeyToolRunDependencySelf      Key = "tool.run.error.dependency_self"
	KeyToolRunDependencyDuplicate Key = "tool.run.error.dependency_duplicate"
	KeyToolRunDependencyCycle     Key = "tool.run.error.dependency_cycle"
	KeyToolRunOutputBounds        Key = "tool.run.error.output_bounds"
	KeyToolRunApprovalRequired    Key = "tool.run.error.approval_required"
	KeyToolRunPlanModeBlocked     Key = "tool.run.error.plan_mode_blocked"
	KeyToolRunSandboxUnavailable  Key = "tool.run.error.sandbox_unavailable"
	KeyToolRunCommandBuildFailed  Key = "tool.run.error.command_build_failed"
	KeyToolRunTypedResultInvalid  Key = "tool.run.error.typed_result_invalid"
	KeyToolRunSkippedAfterPatch   Key = "tool.run.error.skipped_after_patch"
	KeyToolRunRevisionChanged     Key = "tool.run.error.revision_changed"
	KeyToolRunPatchCommitRequired Key = "tool.run.error.patch_commit_required"
	KeyToolRunCommittedUnverified Key = "tool.run.result.committed_unverified"
	KeyToolRunSealReceiptMissing  Key = "tool.run.result.seal_receipt_missing"
	KeyToolRunSealPlanUnsupported Key = "tool.run.result.seal_plan_unsupported"
	KeyToolRunSealSafetyFailed    Key = "tool.run.result.seal_safety_failed"

	KeyToolRunPermissionStep Key = "tool.run.permission.step"
	KeyToolRunSummary        Key = "tool.run.result.summary"
	KeyToolRunStepResult     Key = "tool.run.result.step"
	KeyToolRunStdout         Key = "tool.run.result.stdout"
	KeyToolRunStderr         Key = "tool.run.result.stderr"
	KeyToolRunOutputOmitted  Key = "tool.run.result.output_omitted"
)

var toolRunKeys = [...]Key{
	KeyToolRunDescription, KeyToolRunSchemaSteps, KeyToolRunSchemaStepID,
	KeyToolRunSchemaArgv, KeyToolRunSchemaShellScript, KeyToolRunSchemaCWD,
	KeyToolRunSchemaTimeout, KeyToolRunSchemaDependsOn, KeyToolRunSchemaFailFast,
	KeyToolRunSchemaHead, KeyToolRunSchemaTail, KeyToolRunSchemaMaxChars,
	KeyToolRunSchemaRequiresPatchCommit,
	KeyToolRunInvalidInput, KeyToolRunStepsRequired, KeyToolRunTooManySteps,
	KeyToolRunStepIDRequired, KeyToolRunStepIDInvalid, KeyToolRunStepIDDuplicate,
	KeyToolRunCommandChoice, KeyToolRunArgumentInvalid, KeyToolRunCWDInvalid,
	KeyToolRunCWDNotDirectory, KeyToolRunTimeoutInvalid, KeyToolRunDependencyUnknown,
	KeyToolRunDependencySelf, KeyToolRunDependencyDuplicate, KeyToolRunDependencyCycle,
	KeyToolRunOutputBounds, KeyToolRunApprovalRequired, KeyToolRunPlanModeBlocked,
	KeyToolRunSandboxUnavailable, KeyToolRunCommandBuildFailed,
	KeyToolRunTypedResultInvalid, KeyToolRunPermissionStep, KeyToolRunSummary,
	KeyToolRunSkippedAfterPatch, KeyToolRunRevisionChanged,
	KeyToolRunPatchCommitRequired, KeyToolRunCommittedUnverified,
	KeyToolRunSealReceiptMissing, KeyToolRunSealPlanUnsupported,
	KeyToolRunSealSafetyFailed,
	KeyToolRunStepResult, KeyToolRunStdout, KeyToolRunStderr, KeyToolRunOutputOmitted,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolRunDescription,
		"Execute an immutable dependency graph of commands with one safety preflight, bounded output, and parallel read-only steps.",
		"执行不可变的命令依赖图：统一进行一次安全预检、严格限制输出，并并行运行只读步骤。",
		"Führt einen unveränderlichen Befehls-Abhängigkeitsgraphen mit einer Sicherheitsvorprüfung, begrenzter Ausgabe und parallelen Nur-Lese-Schritten aus.",
		"不変のコマンド依存グラフを、1 回の安全事前検査、制限付き出力、読み取り専用ステップの並列実行で処理します。",
		"변경 불가능한 명령 종속성 그래프를 한 번의 안전 사전 검사, 제한된 출력, 읽기 전용 단계의 병렬 실행으로 처리합니다.",
		"Выполняет неизменяемый граф зависимостей команд с единой проверкой безопасности, ограниченным выводом и параллельными шагами только для чтения.")
	add(KeyToolRunSchemaSteps,
		"Commands and their dependencies. Each step must use exactly one of argv or shell_script.",
		"命令及其依赖关系。每个步骤必须且只能使用 argv 或 shell_script 之一。",
		"Befehle und ihre Abhängigkeiten. Jeder Schritt muss genau eines von argv oder shell_script verwenden.",
		"コマンドとその依存関係。各ステップでは argv または shell_script のどちらか一方だけを指定します。",
		"명령과 종속성입니다. 각 단계는 argv 또는 shell_script 중 정확히 하나만 사용해야 합니다.",
		"Команды и их зависимости. Каждый шаг должен использовать только одно из полей argv или shell_script.")
	add(KeyToolRunSchemaStepID,
		"Stable step ID used by depends_on.", "depends_on 引用的稳定步骤 ID。",
		"Stabile Schritt-ID für depends_on.", "depends_on から参照する安定したステップ ID。",
		"depends_on에서 참조하는 안정적인 단계 ID입니다.", "Стабильный идентификатор шага для depends_on.")
	add(KeyToolRunSchemaArgv,
		"Executable and arguments passed directly without a shell.", "不经过 shell、直接传递的可执行文件和参数。",
		"Programm und Argumente, die ohne Shell direkt übergeben werden.", "シェルを介さず直接渡す実行ファイルと引数。",
		"셸 없이 직접 전달할 실행 파일과 인수입니다.", "Исполняемый файл и аргументы, передаваемые напрямую без оболочки.")
	add(KeyToolRunSchemaShellScript,
		"Bash script executed with pipefail, no startup files, and the same safety policy as Bash.",
		"使用 pipefail、禁用启动文件，并按 Bash 相同安全策略执行的脚本。",
		"Bash-Skript mit pipefail, ohne Startdateien und mit derselben Sicherheitsrichtlinie wie Bash.",
		"pipefail を有効にし、起動ファイルを読まず、Bash と同じ安全ポリシーで実行するスクリプト。",
		"pipefail을 사용하고 시작 파일을 읽지 않으며 Bash와 같은 안전 정책으로 실행하는 스크립트입니다.",
		"Сценарий Bash, выполняемый с pipefail, без файлов запуска и с той же политикой безопасности, что и Bash.")
	add(KeyToolRunSchemaCWD,
		"Step working directory, relative to the workspace by default.", "步骤工作目录；相对路径默认以工作区为基准。",
		"Arbeitsverzeichnis des Schritts; relative Pfade beziehen sich standardmäßig auf den Arbeitsbereich.", "ステップの作業ディレクトリ。相対パスは既定でワークスペース基準です。",
		"단계 작업 디렉터리이며 상대 경로는 기본적으로 작업 공간을 기준으로 합니다.", "Рабочий каталог шага; относительный путь по умолчанию отсчитывается от рабочей области.")
	add(KeyToolRunSchemaTimeout,
		"Per-step timeout in milliseconds.", "每个步骤的超时时间（毫秒）。",
		"Zeitlimit pro Schritt in Millisekunden.", "ステップごとのタイムアウト（ミリ秒）。",
		"단계별 제한 시간(밀리초)입니다.", "Тайм-аут каждого шага в миллисекундах.")
	add(KeyToolRunSchemaDependsOn,
		"Step IDs that must finish successfully before this step starts.", "本步骤启动前必须成功完成的步骤 ID。",
		"Schritt-IDs, die vor dem Start dieses Schritts erfolgreich beendet sein müssen.", "このステップの開始前に成功している必要があるステップ ID。",
		"이 단계를 시작하기 전에 성공적으로 끝나야 하는 단계 ID입니다.", "Идентификаторы шагов, которые должны успешно завершиться до запуска этого шага.")
	add(KeyToolRunSchemaFailFast,
		"Cancel running work and skip pending work after the first failed step.", "首个步骤失败后取消正在运行的工作并跳过待执行工作。",
		"Bricht laufende Arbeit ab und überspringt ausstehende Arbeit nach dem ersten fehlgeschlagenen Schritt.", "最初のステップ失敗後、実行中の処理を中止し、未実行の処理をスキップします。",
		"첫 단계 실패 후 실행 중인 작업을 취소하고 대기 중인 작업을 건너뜁니다.", "Отменяет выполняемую и пропускает ожидающую работу после первой ошибки шага.")
	add(KeyToolRunSchemaHead,
		"Maximum leading lines retained for each output stream.", "每个输出流保留的开头行数上限。",
		"Höchstzahl der beibehaltenen Anfangszeilen je Ausgabestrom.", "各出力ストリームで保持する先頭行数の上限。",
		"각 출력 스트림에서 유지할 앞부분 줄 수의 상한입니다.", "Максимальное число сохраняемых начальных строк каждого потока вывода.")
	add(KeyToolRunSchemaTail,
		"Maximum trailing lines retained for each output stream.", "每个输出流保留的末尾行数上限。",
		"Höchstzahl der beibehaltenen Schlusszeilen je Ausgabestrom.", "各出力ストリームで保持する末尾行数の上限。",
		"각 출력 스트림에서 유지할 뒷부분 줄 수의 상한입니다.", "Максимальное число сохраняемых конечных строк каждого потока вывода.")
	add(KeyToolRunSchemaMaxChars,
		"Hard character budget shared by all step output excerpts.", "所有步骤输出摘要共享的严格字符预算。",
		"Festes Zeichenbudget für die Ausgabeauszüge aller Schritte.", "全ステップの出力抜粋で共有する厳密な文字数上限。",
		"모든 단계 출력 발췌문이 공유하는 엄격한 문자 예산입니다.", "Жёсткий общий лимит символов для фрагментов вывода всех шагов.")
	add(KeyToolRunSchemaRequiresPatchCommit,
		"Within one assistant response, an immediately preceding ApplyPatch is bound by default. Set false only when this Run is deliberately independent; omission elsewhere creates no scheduling dependency.",
		"在同一条助手响应中，默认绑定紧邻此 Run 的前一个 ApplyPatch。仅当此 Run 确实独立时才设为 false；在其他位置省略该字段不会建立调度依赖。",
		"Innerhalb einer einzelnen Assistentenantwort wird ein unmittelbar vorangehendes ApplyPatch standardmäßig gebunden. Setzen Sie den Wert nur dann auf false, wenn dieser Run bewusst unabhängig ist; wird das Feld an anderer Stelle weggelassen, entsteht keine Planungsabhängigkeit.",
		"同じアシスタント応答内では、直前の ApplyPatch が既定でこの Run に紐付けられます。この Run を意図的に独立させる場合に限り false を指定してください。それ以外の位置で省略してもスケジューリング依存関係は作成されません。",
		"하나의 어시스턴트 응답 안에서는 바로 앞의 ApplyPatch가 기본적으로 이 Run에 바인딩됩니다. 이 Run을 의도적으로 독립 실행할 때만 false로 설정하세요. 다른 위치에서 이 필드를 생략해도 스케줄링 종속 관계는 만들어지지 않습니다.",
		"В пределах одного ответа ассистента непосредственно предшествующий ApplyPatch по умолчанию привязывается к этому Run. Указывайте false только для намеренно независимого Run; пропуск поля в другом месте не создаёт зависимости планирования.")

	add(KeyToolRunInvalidInput, "Run input is invalid.", "Run 输入无效。", "Die Run-Eingabe ist ungültig.", "Run の入力が無効です。", "Run 입력이 올바르지 않습니다.", "Недопустимые входные данные Run.")
	add(KeyToolRunStepsRequired, "Run requires at least one step.", "Run 至少需要一个步骤。", "Run benötigt mindestens einen Schritt.", "Run には 1 つ以上のステップが必要です。", "Run에는 단계가 하나 이상 필요합니다.", "Для Run требуется хотя бы один шаг.")
	add(KeyToolRunTooManySteps, "Run accepts at most %d steps.", "Run 最多接受 %d 个步骤。", "Run akzeptiert höchstens %d Schritte.", "Run で指定できるステップは最大 %d 個です。", "Run은 최대 %d개 단계까지 허용합니다.", "Run принимает не более %d шагов.")
	add(KeyToolRunStepIDRequired, "Step %d requires an ID.", "第 %d 个步骤需要 ID。", "Schritt %d benötigt eine ID.", "ステップ %d には ID が必要です。", "%d번째 단계에는 ID가 필요합니다.", "Для шага %d требуется идентификатор.")
	add(KeyToolRunStepIDInvalid, "Step ID %q is invalid.", "步骤 ID %q 无效。", "Die Schritt-ID %q ist ungültig.", "ステップ ID %q は無効です。", "단계 ID %q이(가) 올바르지 않습니다.", "Недопустимый идентификатор шага %q.")
	add(KeyToolRunStepIDDuplicate, "Step ID %q is duplicated.", "步骤 ID %q 重复。", "Die Schritt-ID %q ist doppelt vorhanden.", "ステップ ID %q が重複しています。", "단계 ID %q이(가) 중복되었습니다.", "Идентификатор шага %q повторяется.")
	add(KeyToolRunCommandChoice, "Step %q must use exactly one of argv or shell_script.", "步骤 %q 必须且只能使用 argv 或 shell_script 之一。", "Schritt %q muss genau eines von argv oder shell_script verwenden.", "ステップ %q では argv または shell_script のどちらか一方だけを指定してください。", "%q 단계는 argv 또는 shell_script 중 정확히 하나만 사용해야 합니다.", "Шаг %q должен использовать только одно из полей argv или shell_script.")
	add(KeyToolRunArgumentInvalid, "Step %q contains an invalid argument at position %d.", "步骤 %q 在位置 %d 包含无效参数。", "Schritt %q enthält an Position %d ein ungültiges Argument.", "ステップ %q の位置 %d に無効な引数があります。", "%q 단계의 %d 위치에 올바르지 않은 인수가 있습니다.", "Шаг %q содержит недопустимый аргумент в позиции %d.")
	add(KeyToolRunCWDInvalid, "Step %q has an invalid working directory: %s", "步骤 %q 的工作目录无效：%s", "Schritt %q hat ein ungültiges Arbeitsverzeichnis: %s", "ステップ %q の作業ディレクトリが無効です: %s", "%q 단계의 작업 디렉터리가 올바르지 않습니다: %s", "Шаг %q содержит недопустимый рабочий каталог: %s")
	add(KeyToolRunCWDNotDirectory, "Step %q working directory is not a directory: %s", "步骤 %q 的工作目录不是目录：%s", "Das Arbeitsverzeichnis von Schritt %q ist kein Verzeichnis: %s", "ステップ %q の作業ディレクトリはディレクトリではありません: %s", "%q 단계의 작업 디렉터리가 디렉터리가 아닙니다: %s", "Рабочий каталог шага %q не является каталогом: %s")
	add(KeyToolRunTimeoutInvalid, "Step %q timeout must be between 1 and %d milliseconds.", "步骤 %q 的超时必须介于 1 到 %d 毫秒之间。", "Das Zeitlimit von Schritt %q muss zwischen 1 und %d Millisekunden liegen.", "ステップ %q のタイムアウトは 1～%d ミリ秒で指定してください。", "%q 단계의 제한 시간은 1~%d밀리초여야 합니다.", "Тайм-аут шага %q должен быть от 1 до %d миллисекунд.")
	add(KeyToolRunDependencyUnknown, "Step %q depends on unknown step %q.", "步骤 %q 依赖未知步骤 %q。", "Schritt %q hängt vom unbekannten Schritt %q ab.", "ステップ %q は不明なステップ %q に依存しています。", "%q 단계가 알 수 없는 %q 단계에 종속됩니다.", "Шаг %q зависит от неизвестного шага %q.")
	add(KeyToolRunDependencySelf, "Step %q cannot depend on itself.", "步骤 %q 不能依赖自身。", "Schritt %q kann nicht von sich selbst abhängen.", "ステップ %q を自身に依存させることはできません。", "%q 단계는 자기 자신에 종속될 수 없습니다.", "Шаг %q не может зависеть от самого себя.")
	add(KeyToolRunDependencyDuplicate, "Step %q lists dependency %q more than once.", "步骤 %q 多次列出依赖 %q。", "Schritt %q führt die Abhängigkeit %q mehrfach auf.", "ステップ %q で依存先 %q が重複しています。", "%q 단계에 %q 종속성이 두 번 이상 나열되었습니다.", "Шаг %q содержит зависимость %q несколько раз.")
	add(KeyToolRunDependencyCycle, "Run dependencies contain a cycle.", "Run 依赖关系中存在环。", "Die Run-Abhängigkeiten enthalten einen Zyklus.", "Run の依存関係に循環があります。", "Run 종속성에 순환이 있습니다.", "Зависимости Run содержат цикл.")
	add(KeyToolRunOutputBounds, "head, tail, and max_chars exceed the supported output bounds.", "head、tail 或 max_chars 超出支持的输出范围。", "head, tail oder max_chars überschreiten die unterstützten Ausgabegrenzen.", "head、tail、max_chars が対応する出力範囲を超えています。", "head, tail 또는 max_chars가 지원되는 출력 범위를 벗어났습니다.", "head, tail или max_chars выходят за допустимые границы вывода.")
	add(KeyToolRunApprovalRequired, "Run execution requires a valid preflight approval.", "Run 执行需要有效的预检批准。", "Die Run-Ausführung benötigt eine gültige Vorabgenehmigung.", "Run の実行には有効な事前承認が必要です。", "Run 실행에는 유효한 사전 승인이 필요합니다.", "Для выполнения Run требуется действительное предварительное разрешение.")
	add(KeyToolRunPlanModeBlocked, "Run cannot execute while plan mode is active.", "计划模式启用时不能执行 Run。", "Run kann im Planmodus nicht ausgeführt werden.", "プランモード中は Run を実行できません。", "계획 모드에서는 Run을 실행할 수 없습니다.", "Run нельзя выполнять в режиме планирования.")
	add(KeyToolRunSandboxUnavailable, "The approved sandbox authority is no longer available.", "已批准的沙箱执行权限已不可用。", "Die genehmigte Sandbox-Autorität ist nicht mehr verfügbar.", "承認済みのサンドボックス実行権限が利用できなくなりました。", "승인된 샌드박스 실행 권한을 더 이상 사용할 수 없습니다.", "Одобренная среда изоляции больше недоступна.")
	add(KeyToolRunCommandBuildFailed, "Step %q could not be prepared for execution.", "无法为步骤 %q 准备执行命令。", "Schritt %q konnte nicht zur Ausführung vorbereitet werden.", "ステップ %q を実行用に準備できませんでした。", "%q 단계를 실행할 수 있도록 준비하지 못했습니다.", "Не удалось подготовить шаг %q к выполнению.")
	add(KeyToolRunTypedResultInvalid, "Run returned an invalid structured result.", "Run 返回了无效的结构化结果。", "Run hat ein ungültiges strukturiertes Ergebnis zurückgegeben.", "Run が無効な構造化結果を返しました。", "Run이 올바르지 않은 구조화 결과를 반환했습니다.", "Run вернул недопустимый структурированный результат.")
	add(KeyToolRunSkippedAfterPatch,
		"Run was skipped because the preceding ApplyPatch did not commit a certifiable workspace revision.",
		"已跳过 Run，因为前一个 ApplyPatch 未提交可认证的工作区版本。",
		"Run wurde übersprungen, weil das vorangehende ApplyPatch keine bestätigbare Workspace-Revision committet hat.",
		"直前の ApplyPatch が証明可能な workspace revision を commit しなかったため、Run をスキップしました。",
		"앞선 ApplyPatch가 인증 가능한 workspace revision을 commit하지 않아 Run을 건너뛰었습니다.",
		"Run пропущен: предшествующий ApplyPatch не зафиксировал подтверждаемую ревизию рабочей области.")
	add(KeyToolRunRevisionChanged,
		"Run did not verify the patch because the workspace revision changed before or during verification.",
		"Run 未能验证补丁，因为工作区版本在验证前或验证期间发生了变化。",
		"Run hat den Patch nicht verifiziert, weil sich die Workspace-Revision vor oder während der Verifizierung geändert hat.",
		"検証前または検証中に workspace revision が変化したため、Run はパッチを検証できませんでした。",
		"검증 전이나 도중에 workspace revision이 변경되어 Run이 패치를 검증하지 못했습니다.",
		"Run не подтвердил patch: ревизия рабочей области изменилась до или во время проверки.")
	add(KeyToolRunPatchCommitRequired,
		"Run was not started because its required ApplyPatch commit receipt is unavailable.",
		"Run 未启动，因为所需的 ApplyPatch 提交回执不可用。",
		"Run wurde nicht gestartet, weil der erforderliche ApplyPatch-Commit-Beleg nicht verfügbar ist.",
		"必要な ApplyPatch commit receipt がないため、Run は開始されませんでした。",
		"필요한 ApplyPatch commit receipt를 사용할 수 없어 Run을 시작하지 않았습니다.",
		"Run не запущен: отсутствует требуемая квитанция commit от ApplyPatch.")
	add(KeyToolRunCommittedUnverified,
		"Run may have changed the workspace. Its checks are not verification evidence until the resulting revision is sealed and checked by a read-only Run.",
		"Run 可能已更改工作区。在生成的版本被封存并由只读 Run 检查前，其中的检查不构成验证证据。",
		"Run hat möglicherweise den Workspace geändert. Seine Prüfungen gelten erst als Verifizierungsnachweis, nachdem die resultierende Revision versiegelt und von einem schreibgeschützten Run geprüft wurde.",
		"Run が workspace を変更した可能性があります。生成された revision を seal し、読み取り専用 Run で確認するまでは、この Run のチェックは検証証拠になりません。",
		"Run이 workspace를 변경했을 수 있습니다. 생성된 revision을 seal하고 읽기 전용 Run으로 검사하기 전까지 이 Run의 검사는 검증 증거가 아닙니다.",
		"Run мог изменить рабочую область. Его проверки не считаются доказательством, пока итоговая ревизия не запечатана и не проверена Run только для чтения.")
	add(KeyToolRunSealReceiptMissing,
		"This Run had no bound ApplyPatch receipt, so possible workspace changes could not be sealed. Inspect the diff. If a real source change remains, make it with ApplyPatch, then verify with a read-only Run. A no-op patch cannot issue a receipt; if no change remains, report that this query lacks adoption or sealing authority.",
		"此 Run 未绑定 ApplyPatch 回执，因此无法封存可能产生的工作区变更。请检查 diff。若仍需实际修改源代码，请用 ApplyPatch 完成，再用只读 Run 验证。空操作补丁无法签发回执；若已无需修改，请报告本次查询缺少采纳或封存该状态的权限。",
		"Dieser Run hatte keinen gebundenen ApplyPatch-Beleg; mögliche Workspace-Änderungen konnten daher nicht versiegelt werden. Prüfen Sie den Diff. Falls noch eine echte Quelländerung nötig ist, führen Sie sie mit ApplyPatch aus und verifizieren Sie danach mit einem schreibgeschützten Run. Ein No-op-Patch kann keinen Beleg ausstellen; falls keine Änderung mehr nötig ist, melden Sie, dass dieser Abfrage die Berechtigung zur Übernahme oder Versiegelung fehlt.",
		"この Run には ApplyPatch の receipt が紐付いていなかったため、workspace に生じた可能性のある変更を seal できませんでした。diff を確認してください。実際のソース変更がまだ必要なら ApplyPatch で行い、その後に読み取り専用 Run で検証してください。no-op patch では receipt を発行できません。変更が不要なら、この query には現在の状態を adopt または seal する権限がないことを報告してください。",
		"이 Run에는 바인딩된 ApplyPatch receipt가 없어 workspace에 생겼을 수 있는 변경을 seal할 수 없습니다. diff를 확인하세요. 실제 소스 변경이 남아 있다면 ApplyPatch로 수행한 다음 읽기 전용 Run으로 검증하세요. no-op patch는 receipt를 발급할 수 없습니다. 변경이 남아 있지 않다면 이 query에 현재 상태를 adopt하거나 seal할 권한이 없다고 보고하세요.",
		"У этого Run не было привязанной квитанции ApplyPatch, поэтому возможные изменения рабочей области нельзя было запечатать. Просмотрите diff. Если ещё требуется реальное изменение исходников, внесите его через ApplyPatch, затем проверьте с помощью Run только для чтения. Пустой патч не может выдать квитанцию; если изменений больше не требуется, сообщите, что у этого запроса нет полномочий принять или запечатать текущее состояние.")
	add(KeyToolRunSealPlanUnsupported,
		"This Run graph contains a workspace-writing step that cannot be sealed. Move source edits to ApplyPatch; keep Run limited to tests, builds, static checks, and read-only observations.",
		"此 Run 图包含无法封存的工作区写入步骤。请把源代码编辑移到 ApplyPatch，并将 Run 限定为测试、构建、静态检查和只读观察。",
		"Dieser Run-Graph enthält einen schreibenden Workspace-Schritt, der nicht versiegelt werden kann. Verschieben Sie Quelländerungen nach ApplyPatch und beschränken Sie Run auf Tests, Builds, statische Prüfungen und schreibgeschützte Beobachtungen.",
		"この Run グラフには seal できない workspace 書き込みステップが含まれています。ソース編集は ApplyPatch に移し、Run はテスト、ビルド、静的チェック、読み取り専用の確認に限定してください。",
		"이 Run 그래프에는 seal할 수 없는 workspace 쓰기 단계가 있습니다. 소스 편집은 ApplyPatch로 옮기고 Run은 테스트, 빌드, 정적 검사, 읽기 전용 확인에만 사용하세요.",
		"Граф Run содержит шаг записи в рабочую область, который нельзя запечатать. Перенесите правки исходников в ApplyPatch, а Run используйте только для тестов, сборки, статических проверок и операций чтения.")
	add(KeyToolRunSealSafetyFailed,
		"Run could not seal the revision because safety check %s failed. Inspect the diff. If a real source change remains, make it with ApplyPatch, then verify with a read-only Run. Do not invent a no-op change; if no change remains, report missing adoption or sealing authority.",
		"Run 无法封存该版本，因为安全检查 %s 未通过。请检查 diff。若仍需实际修改源代码，请用 ApplyPatch 完成，再用只读 Run 验证。不要伪造空操作变更；若已无需修改，请报告缺少采纳或封存该状态的权限。",
		"Run konnte die Revision nicht versiegeln, weil die Sicherheitsprüfung %s fehlgeschlagen ist. Prüfen Sie den Diff. Falls noch eine echte Quelländerung nötig ist, führen Sie sie mit ApplyPatch aus und verifizieren Sie danach mit einem schreibgeschützten Run. Erfinden Sie keine No-op-Änderung; falls keine Änderung mehr nötig ist, melden Sie die fehlende Berechtigung zur Übernahme oder Versiegelung.",
		"安全チェック %s に失敗したため、Run は revision を seal できませんでした。diff を確認してください。実際のソース変更がまだ必要なら ApplyPatch で行い、その後に読み取り専用 Run で検証してください。no-op の変更を作らないでください。変更が不要なら、adopt または seal する権限がないことを報告してください。",
		"안전 검사 %s에 실패하여 Run이 revision을 seal하지 못했습니다. diff를 확인하세요. 실제 소스 변경이 남아 있다면 ApplyPatch로 수행한 다음 읽기 전용 Run으로 검증하세요. no-op 변경을 만들지 마세요. 변경이 남아 있지 않다면 adopt 또는 seal 권한이 없다고 보고하세요.",
		"Run не смог запечатать ревизию: проверка безопасности %s завершилась неудачно. Просмотрите diff. Если ещё требуется реальное изменение исходников, внесите его через ApplyPatch, затем проверьте с помощью Run только для чтения. Не создавайте фиктивное пустое изменение; если изменений больше не требуется, сообщите об отсутствии полномочий принять или запечатать состояние.")

	add(KeyToolRunPermissionStep, "Step %q requires permission: %s", "步骤 %q 需要权限：%s", "Schritt %q benötigt eine Berechtigung: %s", "ステップ %q には権限が必要です: %s", "%q 단계에 권한이 필요합니다: %s", "Для шага %q требуется разрешение: %s")
	add(KeyToolRunSummary, "Run finished: %d succeeded, %d failed, %d timed out, %d cancelled, %d skipped.", "Run 已完成：%d 个成功，%d 个失败，%d 个超时，%d 个取消，%d 个跳过。", "Run beendet: %d erfolgreich, %d fehlgeschlagen, %d mit Zeitüberschreitung, %d abgebrochen, %d übersprungen.", "Run 完了: 成功 %d、失敗 %d、タイムアウト %d、キャンセル %d、スキップ %d。", "Run 완료: 성공 %d개, 실패 %d개, 시간 초과 %d개, 취소 %d개, 건너뜀 %d개.", "Run завершён: успешно — %d, с ошибкой — %d, по тайм-ауту — %d, отменено — %d, пропущено — %d.")
	add(KeyToolRunStepResult, "[%s] status=%s exit=%d duration_ms=%d truncated=%t effect=%s", "[%s] 状态=%s 退出码=%d 耗时毫秒=%d 已截断=%t 影响=%s", "[%s] Status=%s Exit=%d Dauer_ms=%d gekürzt=%t Effekt=%s", "[%s] 状態=%s 終了=%d 所要_ms=%d 切り詰め=%t 効果=%s", "[%s] 상태=%s 종료=%d 소요_ms=%d 잘림=%t 영향=%s", "[%s] статус=%s код=%d длительность_мс=%d усечено=%t эффект=%s")
	add(KeyToolRunStdout, "stdout:", "标准输出：", "Standardausgabe:", "標準出力:", "표준 출력:", "стандартный вывод:")
	add(KeyToolRunStderr, "stderr:", "标准错误：", "Standardfehler:", "標準エラー:", "표준 오류:", "стандартный поток ошибок:")
	add(KeyToolRunOutputOmitted, "... %d bytes omitted ...", "……已省略 %d 字节……", "... %d Bytes ausgelassen ...", "... %d バイト省略 ...", "... %d바이트 생략 ...", "... пропущено %d байт ...")
}
