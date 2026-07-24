package i18n

// Semantic copy for configuration, permissions, setup, and diagnostics commands.
const (
	KeyAdminReadSettingsError           Key = "admin.settings.read_error"
	KeyAdminWriteSettingsError          Key = "admin.settings.write_error"
	KeyConfigUsageGet                   Key = "command.config.usage.get"
	KeyConfigUsageSet                   Key = "command.config.usage.set"
	KeyConfigUsage                      Key = "command.config.usage"
	KeyConfigNoSettings                 Key = "command.config.no_settings"
	KeyConfigSettings                   Key = "command.config.settings"
	KeyConfigKeyMissing                 Key = "command.config.key_missing"
	KeyConfigUnknownKey                 Key = "command.config.unknown_key"
	KeyConfigInvalidCacheRoutingMode    Key = "command.config.invalid_cache_routing_mode"
	KeyConfigValue                      Key = "command.config.value"
	KeyConfigSet                        Key = "command.config.set"
	KeyPermissionsUsageAllow            Key = "command.permissions.usage.allow"
	KeyPermissionsUsageDeny             Key = "command.permissions.usage.deny"
	KeyPermissionsUsage                 Key = "command.permissions.usage"
	KeyPermissionsTitle                 Key = "command.permissions.title"
	KeyPermissionsNone                  Key = "command.permissions.none"
	KeyPermissionsEdit                  Key = "command.permissions.edit"
	KeyPermissionsAllowed               Key = "command.permissions.allowed"
	KeyPermissionsDenied                Key = "command.permissions.denied"
	KeyPermissionsAllowItem             Key = "command.permissions.allow_item"
	KeyPermissionsDenyItem              Key = "command.permissions.deny_item"
	KeyPermissionsUpdated               Key = "command.permissions.updated"
	KeyInitCreateDirError               Key = "command.init.create_dir_error"
	KeyInitCreateFileError              Key = "command.init.create_file_error"
	KeyInitCreateSettingsError          Key = "command.init.create_settings_error"
	KeyInitReport                       Key = "command.init.report"
	KeyInitCreated                      Key = "command.init.created"
	KeyInitExists                       Key = "command.init.exists"
	KeyDoctorResult                     Key = "command.doctor.result"
	KeyDoctorResolveFailures            Key = "command.doctor.resolve_failures"
	KeyDoctorLabelCredentials           Key = "command.doctor.label.credentials"
	KeyDoctorLabelModel                 Key = "command.doctor.label.model"
	KeyDoctorLabelGit                   Key = "command.doctor.label.git"
	KeyDoctorLabelSandbox               Key = "command.doctor.label.sandbox"
	KeyDoctorLabelMCP                   Key = "command.doctor.label.mcp"
	KeyDoctorLabelNode                  Key = "command.doctor.label.node"
	KeyDoctorLabelDisk                  Key = "command.doctor.label.disk"
	KeyDoctorLabelConfig                Key = "command.doctor.label.config"
	KeyDoctorLabelOllama                Key = "command.doctor.label.ollama"
	KeyDoctorCredentialState            Key = "command.doctor.credential_state"
	KeyDoctorCredentialEnv              Key = "command.doctor.credential_env"
	KeyDoctorCredentialAuthToken        Key = "command.doctor.credential_auth_token"
	KeyDoctorCredentialStore            Key = "command.doctor.credential_store"
	KeyDoctorCredentialOAuth            Key = "command.doctor.credential_oauth"
	KeyDoctorCredentialImported         Key = "command.doctor.credential_imported"
	KeyDoctorCredentialAWS              Key = "command.doctor.credential_aws"
	KeyDoctorCredentialGCP              Key = "command.doctor.credential_gcp"
	KeyDoctorCredentialAnthropicMissing Key = "command.doctor.credential_anthropic_missing"
	KeyDoctorCredentialMissing          Key = "command.doctor.credential_missing"
	KeyDoctorNoModel                    Key = "command.doctor.no_model"
	KeyDoctorContextWindow              Key = "command.doctor.context_window"
	KeyDoctorReasoning                  Key = "command.doctor.reasoning"
	KeyDoctorCustomModel                Key = "command.doctor.custom_model"
	KeyDoctorOllamaUnreachable          Key = "command.doctor.ollama_unreachable"
	KeyDoctorOllamaHTTP                 Key = "command.doctor.ollama_http"
	KeyDoctorOllamaRunning              Key = "command.doctor.ollama_running"
	KeyDoctorGitMissing                 Key = "command.doctor.git_missing"
	KeyDoctorGitRepo                    Key = "command.doctor.git_repo"
	KeyDoctorGitNotRepo                 Key = "command.doctor.git_not_repo"
	KeyDoctorSandboxMissing             Key = "command.doctor.sandbox_missing"
	KeyDoctorSandboxAvailable           Key = "command.doctor.sandbox_available"
	KeyDoctorSandboxUnsupported         Key = "command.doctor.sandbox_unsupported"
	KeyDoctorNodeMissing                Key = "command.doctor.node_missing"
	KeyDoctorNodeUnknown                Key = "command.doctor.node_unknown"
	KeyDoctorConfigUnreadable           Key = "command.doctor.config_unreadable"
	KeyDoctorConfigInvalid              Key = "command.doctor.config_invalid"
	KeyDoctorConfigNone                 Key = "command.doctor.config_none"
	KeyDoctorConfigValid                Key = "command.doctor.config_valid"
	KeyDoctorDiskFree                   Key = "command.doctor.disk_free"
	KeyDoctorDiskLow                    Key = "command.doctor.disk_low"
	KeyDoctorDiskStatError              Key = "command.doctor.disk_stat_error"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = commandCore(en, zh, de, ja, ko, ru)
	}
	add(KeyAdminReadSettingsError, "Error reading settings: %v\n", "读取设置时出错：%v\n", "Fehler beim Lesen der Einstellungen: %v\n", "設定の読み取りエラー: %v\n", "설정 읽기 오류: %v\n", "Ошибка чтения настроек: %v\n")
	add(KeyAdminWriteSettingsError, "Error writing settings: %v\n", "写入设置时出错：%v\n", "Fehler beim Schreiben der Einstellungen: %v\n", "設定の書き込みエラー: %v\n", "설정 쓰기 오류: %v\n", "Ошибка записи настроек: %v\n")
	add(KeyConfigUsageGet, "Usage: /config get <key>\n", "用法：/config get <key>\n", "Verwendung: /config get <key>\n", "使い方: /config get <key>\n", "사용법: /config get <key>\n", "Использование: /config get <key>\n")
	add(KeyConfigUsageSet, "Usage: /config set <key> <value>\n", "用法：/config set <key> <value>\n", "Verwendung: /config set <key> <value>\n", "使い方: /config set <key> <value>\n", "사용법: /config set <key> <value>\n", "Использование: /config set <key> <value>\n")
	add(KeyConfigUsage, "Usage: /config [list|get <key>|set <key> <value>]\n", "用法：/config [list|get <key>|set <key> <value>]\n", "Verwendung: /config [list|get <key>|set <key> <value>]\n", "使い方: /config [list|get <key>|set <key> <value>]\n", "사용법: /config [list|get <key>|set <key> <value>]\n", "Использование: /config [list|get <key>|set <key> <value>]\n")
	add(KeyConfigNoSettings, "No settings found (checked %s).\n", "未找到设置（已检查 %s）。\n", "Keine Einstellungen gefunden (%s geprüft).\n", "設定が見つかりません（%s を確認済み）。\n", "설정을 찾을 수 없습니다(%s 확인).\n", "Настройки не найдены (проверено: %s).\n")
	add(KeyConfigSettings, "Settings (%s):\n%s\n", "设置（%s）：\n%s\n", "Einstellungen (%s):\n%s\n", "設定（%s）:\n%s\n", "설정(%s):\n%s\n", "Настройки (%s):\n%s\n")
	add(KeyConfigKeyMissing, "Key %q not found in settings.\n", "设置中未找到键 %q。\n", "Schlüssel %q wurde in den Einstellungen nicht gefunden.\n", "設定にキー %q が見つかりません。\n", "설정에서 키 %q을(를) 찾을 수 없습니다.\n", "Ключ %q не найден в настройках.\n")
	add(KeyConfigUnknownKey, "Error: unknown config key %q. Valid keys: %s\n", "错误：未知配置键 %q。有效键：%s\n", "Fehler: unbekannter Konfigurationsschlüssel %q. Gültige Schlüssel: %s\n", "エラー: 不明な設定キー %q。有効なキー: %s\n", "오류: 알 수 없는 구성 키 %q. 유효한 키: %s\n", "Ошибка: неизвестный ключ конфигурации %q. Допустимые ключи: %s\n")
	add(KeyConfigInvalidCacheRoutingMode, "Error: cacheRoutingMode must be auto, on, or off; got %q.\n", "错误：cacheRoutingMode 必须为 auto、on 或 off；当前为 %q。\n", "Fehler: cacheRoutingMode muss auto, on oder off sein; erhalten: %q.\n", "エラー: cacheRoutingMode は auto、on、off のいずれかである必要があります。現在値: %q。\n", "오류: cacheRoutingMode는 auto, on 또는 off여야 합니다. 현재 값: %q.\n", "Ошибка: cacheRoutingMode должен быть auto, on или off; получено: %q.\n")
	add(KeyConfigValue, "%s = %s\n", "%s = %s\n", "%s = %s\n", "%s = %s\n", "%s = %s\n", "%s = %s\n")
	add(KeyConfigSet, "Set %s = %v in %s\n", "已设置 %s = %v（位置：%s）\n", "Gesetzt: %s = %v in %s\n", "%s = %v を %s に設定しました\n", "%s = %v로 설정했습니다(%s)\n", "Установлено: %s = %v в %s\n")
	add(KeyPermissionsUsageAllow, "Usage: /permissions allow <tool>\n", "用法：/permissions allow <tool>\n", "Verwendung: /permissions allow <tool>\n", "使い方: /permissions allow <tool>\n", "사용법: /permissions allow <tool>\n", "Использование: /permissions allow <tool>\n")
	add(KeyPermissionsUsageDeny, "Usage: /permissions deny <tool>\n", "用法：/permissions deny <tool>\n", "Verwendung: /permissions deny <tool>\n", "使い方: /permissions deny <tool>\n", "사용법: /permissions deny <tool>\n", "Использование: /permissions deny <tool>\n")
	add(KeyPermissionsUsage, "Usage: /permissions [list|allow <tool>|deny <tool>]\n", "用法：/permissions [list|allow <tool>|deny <tool>]\n", "Verwendung: /permissions [list|allow <tool>|deny <tool>]\n", "使い方: /permissions [list|allow <tool>|deny <tool>]\n", "사용법: /permissions [list|allow <tool>|deny <tool>]\n", "Использование: /permissions [list|allow <tool>|deny <tool>]\n")
	add(KeyPermissionsTitle, "Tool Permissions\n", "工具权限\n", "Tool-Berechtigungen\n", "ツールの権限\n", "도구 권한\n", "Разрешения инструментов\n")
	add(KeyPermissionsNone, "  No explicit permissions configured.\n", "  未配置显式权限。\n", "  Keine expliziten Berechtigungen konfiguriert.\n", "  明示的な権限は設定されていません。\n", "  명시적 권한이 구성되지 않았습니다.\n", "  Явные разрешения не настроены.\n")
	add(KeyPermissionsEdit, "  (edit %s to add rules)\n", "  （编辑 %s 以添加规则）\n", "  (Bearbeite %s, um Regeln hinzuzufügen)\n", "  （ルールを追加するには %s を編集）\n", "  (규칙을 추가하려면 %s 편집)\n", "  (измените %s, чтобы добавить правила)\n")
	add(KeyPermissionsAllowed, "  Allowed tools:\n", "  允许的工具：\n", "  Erlaubte Tools:\n", "  許可されたツール:\n", "  허용된 도구:\n", "  Разрешённые инструменты:\n")
	add(KeyPermissionsDenied, "  Denied tools:\n", "  禁止的工具：\n", "  Verbotene Tools:\n", "  拒否されたツール:\n", "  거부된 도구:\n", "  Запрещённые инструменты:\n")
	add(KeyPermissionsAllowItem, "    ✓ %s\n", "    ✓ %s\n", "    ✓ %s\n", "    ✓ %s\n", "    ✓ %s\n", "    ✓ %s\n")
	add(KeyPermissionsDenyItem, "    ✗ %s\n", "    ✗ %s\n", "    ✗ %s\n", "    ✗ %s\n", "    ✗ %s\n", "    ✗ %s\n")
	add(KeyPermissionsUpdated, "Permission updated: %s → %sd in %s\n", "权限已更新：%s → %sd，位置：%s\n", "Berechtigung aktualisiert: %s → %sd in %s\n", "権限を更新しました: %s → %sd（%s）\n", "권한을 업데이트했습니다: %s → %sd(%s)\n", "Разрешение обновлено: %s → %sd в %s\n")
	add(KeyInitCreateDirError, "Error creating %s/: %v\n", "创建 %s/ 时出错：%v\n", "Fehler beim Erstellen von %s/: %v\n", "%s/ の作成エラー: %v\n", "%s/ 생성 오류: %v\n", "Ошибка создания %s/: %v\n")
	add(KeyInitCreateFileError, "Error creating %s: %v\n", "创建 %s 时出错：%v\n", "Fehler beim Erstellen von %s: %v\n", "%s の作成エラー: %v\n", "%s 생성 오류: %v\n", "Ошибка создания %s: %v\n")
	add(KeyInitCreateSettingsError, "Error creating settings.json: %v\n", "创建 settings.json 时出错：%v\n", "Fehler beim Erstellen von settings.json: %v\n", "settings.json の作成エラー: %v\n", "settings.json 생성 오류: %v\n", "Ошибка создания settings.json: %v\n")
	add(KeyInitReport, "Initialised project structure:\n", "已初始化项目结构：\n", "Projektstruktur initialisiert:\n", "プロジェクト構造を初期化しました:\n", "프로젝트 구조를 초기화했습니다:\n", "Структура проекта инициализирована:\n")
	add(KeyInitCreated, "  ✓ created  %s\n", "  ✓ 已创建  %s\n", "  ✓ erstellt  %s\n", "  ✓ 作成済み  %s\n", "  ✓ 생성됨  %s\n", "  ✓ создано  %s\n")
	add(KeyInitExists, "  · exists   %s\n", "  · 已存在  %s\n", "  · vorhanden %s\n", "  · 既存     %s\n", "  · 존재함   %s\n", "  · существует %s\n")
	add(KeyDoctorResult, "%s %s: %s", "%s %s：%s", "%s %s: %s", "%s %s：%s", "%s %s: %s", "%s %s: %s")
	add(KeyDoctorResolveFailures, "Resolve the %d failed diagnostic check(s), then rerun /doctor.", "请解决 %d 个失败的诊断检查，然后重新运行 /doctor。", "Behebe die %d fehlgeschlagenen Diagnoseprüfungen und führe dann /doctor erneut aus.", "%d 件の失敗した診断チェックを解決してから /doctor を再実行してください。", "실패한 진단 검사 %d개를 해결한 뒤 /doctor를 다시 실행하세요.", "Устраните %d неудачных диагностических проверок и повторно запустите /doctor.")
	add(KeyDoctorLabelCredentials, "Credentials", "凭据", "Anmeldedaten", "認証情報", "자격 증명", "Учётные данные")
	add(KeyDoctorLabelModel, "Model", "模型", "Modell", "モデル", "모델", "Модель")
	add(KeyDoctorLabelGit, "Git", "Git", "Git", "Git", "Git", "Git")
	add(KeyDoctorLabelSandbox, "Sandbox", "沙箱", "Sandbox", "サンドボックス", "샌드박스", "Песочница")
	add(KeyDoctorLabelMCP, "MCP", "MCP", "MCP", "MCP", "MCP", "MCP")
	add(KeyDoctorLabelNode, "Node.js", "Node.js", "Node.js", "Node.js", "Node.js", "Node.js")
	add(KeyDoctorLabelDisk, "Disk", "磁盘", "Datenträger", "ディスク", "디스크", "Диск")
	add(KeyDoctorLabelConfig, "Config", "配置", "Konfiguration", "設定", "구성", "Конфигурация")
	add(KeyDoctorLabelOllama, "Ollama Server", "Ollama 服务器", "Ollama-Server", "Ollama サーバー", "Ollama 서버", "Сервер Ollama")
	add(KeyDoctorCredentialState, "%s — %s", "%s — %s", "%s — %s", "%s — %s", "%s — %s", "%s — %s")
	add(KeyDoctorCredentialEnv, "%s — %s set (%s)", "%s — 已设置 %s（%s）", "%s — %s gesetzt (%s)", "%s — %s が設定済み（%s）", "%s — %s 설정됨(%s)", "%s — %s задана (%s)")
	add(KeyDoctorCredentialAuthToken, "%s — ANTHROPIC_AUTH_TOKEN set (%s)", "%s — 已设置 ANTHROPIC_AUTH_TOKEN（%s）", "%s — ANTHROPIC_AUTH_TOKEN gesetzt (%s)", "%s — ANTHROPIC_AUTH_TOKEN が設定済み（%s）", "%s — ANTHROPIC_AUTH_TOKEN 설정됨(%s)", "%s — ANTHROPIC_AUTH_TOKEN задана (%s)")
	add(KeyDoctorCredentialStore, "%s — credential store (%s)", "%s — 凭据存储（%s）", "%s — Anmeldedatenspeicher (%s)", "%s — 認証情報ストア（%s）", "%s — 자격 증명 저장소(%s)", "%s — хранилище учётных данных (%s)")
	add(KeyDoctorCredentialOAuth, "%s — OAuth token configured", "%s — 已配置 OAuth 令牌", "%s — OAuth-Token konfiguriert", "%s — OAuth トークンを設定済み", "%s — OAuth 토큰 구성됨", "%s — токен OAuth настроен")
	add(KeyDoctorCredentialImported, "%s — imported from env (%s)", "%s — 从环境变量导入（%s）", "%s — aus Umgebungsvariable importiert (%s)", "%s — 環境変数からインポート済み（%s）", "%s — 환경 변수에서 가져옴(%s)", "%s — импортировано из окружения (%s)")
	add(KeyDoctorCredentialAWS, "%s — AWS credentials detected", "%s — 检测到 AWS 凭据", "%s — AWS-Anmeldedaten erkannt", "%s — AWS 認証情報を検出", "%s — AWS 자격 증명 감지됨", "%s — обнаружены учётные данные AWS")
	add(KeyDoctorCredentialGCP, "%s — GCP credentials detected", "%s — 检测到 GCP 凭据", "%s — GCP-Anmeldedaten erkannt", "%s — GCP 認証情報を検出", "%s — GCP 자격 증명 감지됨", "%s — обнаружены учётные данные GCP")
	add(KeyDoctorCredentialAnthropicMissing, "%s — ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN not set", "%s — 未设置 ANTHROPIC_API_KEY 或 ANTHROPIC_AUTH_TOKEN", "%s — ANTHROPIC_API_KEY oder ANTHROPIC_AUTH_TOKEN nicht gesetzt", "%s — ANTHROPIC_API_KEY または ANTHROPIC_AUTH_TOKEN が未設定", "%s — ANTHROPIC_API_KEY 또는 ANTHROPIC_AUTH_TOKEN이 설정되지 않음", "%s — ANTHROPIC_API_KEY или ANTHROPIC_AUTH_TOKEN не задан")
	add(KeyDoctorCredentialMissing, "%s — %s not set", "%s — 未设置 %s", "%s — %s nicht gesetzt", "%s — %s が未設定", "%s — %s이(가) 설정되지 않음", "%s — %s не задан")
	add(KeyDoctorNoModel, "no model configured", "未配置模型", "Kein Modell konfiguriert", "モデルが設定されていません", "모델이 구성되지 않았습니다", "Модель не настроена")
	add(KeyDoctorContextWindow, "%s ctx", "%s 上下文", "%s Kontext", "%s コンテキスト", "%s 컨텍스트", "%s контекст")
	add(KeyDoctorReasoning, "reasoning", "推理", "Reasoning", "推論", "추론", "рассуждение")
	add(KeyDoctorCustomModel, "%s/%s (not in catalog — may be custom)", "%s/%s（不在目录中，可能是自定义模型）", "%s/%s (nicht im Katalog — möglicherweise benutzerdefiniert)", "%s/%s（カタログ外 — カスタムの可能性があります）", "%s/%s(카탈로그에 없음 — 사용자 지정일 수 있음)", "%s/%s (нет в каталоге — возможно, пользовательская)")
	add(KeyDoctorOllamaUnreachable, "unreachable at %s (%v)", "%s 无法访问（%v）", "unter %s nicht erreichbar (%v)", "%s で到達できません（%v）", "%s에 연결할 수 없음(%v)", "недоступен по адресу %s (%v)")
	add(KeyDoctorOllamaHTTP, "responded with HTTP %d at %s", "返回 HTTP %d（%s）", "antwortete mit HTTP %d unter %s", "HTTP %d を返しました（%s）", "HTTP %d 응답(%s)", "ответил HTTP %d по адресу %s")
	add(KeyDoctorOllamaRunning, "running at %s", "%s 正在运行", "läuft unter %s", "%s で稼働中", "%s에서 실행 중", "работает по адресу %s")
	add(KeyDoctorGitMissing, "git not found in PATH", "在 PATH 中未找到 git", "git nicht in PATH gefunden", "PATH に git が見つかりません", "PATH에서 git을 찾을 수 없습니다", "git не найден в PATH")
	add(KeyDoctorGitRepo, "v%s, repo detected", "v%s，已检测到仓库", "v%s, Repository erkannt", "v%s、リポジトリを検出", "v%s, 저장소 감지됨", "v%s, репозиторий обнаружен")
	add(KeyDoctorGitNotRepo, "v%s, not a git repo", "v%s，不是 git 仓库", "v%s, kein Git-Repository", "v%s、Git リポジトリではありません", "v%s, Git 저장소가 아님", "v%s, не Git-репозиторий")
	add(KeyDoctorSandboxMissing, "%s not found", "%s 未找到", "%s nicht gefunden", "%s が見つかりません", "%s을(를) 찾을 수 없음", "%s не найден")
	add(KeyDoctorSandboxAvailable, "%s available", "%s 可用", "%s verfügbar", "%s が利用可能", "%s 사용 가능", "%s доступен")
	add(KeyDoctorSandboxUnsupported, "no sandbox support on %s (skipped)", "%s 不支持沙箱（已跳过）", "keine Sandbox-Unterstützung unter %s (übersprungen)", "%s ではサンドボックス未対応（スキップ）", "%s에서는 샌드박스를 지원하지 않음(건너뜀)", "песочница не поддерживается на %s (пропущено)")
	add(KeyDoctorNodeMissing, "node not found in PATH (required for some MCP servers)", "在 PATH 中未找到 node（部分 MCP 服务器需要）", "node nicht in PATH gefunden (für einige MCP-Server erforderlich)", "PATH に node が見つかりません（一部の MCP サーバーで必要）", "PATH에서 node을 찾을 수 없습니다(일부 MCP 서버에 필요)", "node не найден в PATH (требуется для некоторых серверов MCP)")
	add(KeyDoctorNodeUnknown, "found (version unknown)", "已找到（版本未知）", "gefunden (Version unbekannt)", "見つかりました（バージョン不明）", "발견됨(버전 알 수 없음)", "найден (версия неизвестна)")
	add(KeyDoctorConfigUnreadable, "%s (unreadable)", "%s（无法读取）", "%s (nicht lesbar)", "%s（読み取り不可）", "%s(읽을 수 없음)", "%s (нечитаем)")
	add(KeyDoctorConfigInvalid, "%s (invalid JSON)", "%s（无效 JSON）", "%s (ungültiges JSON)", "%s（無効な JSON）", "%s(잘못된 JSON)", "%s (недопустимый JSON)")
	add(KeyDoctorConfigNone, "no config files found (using defaults)", "未找到配置文件（使用默认值）", "Keine Konfigurationsdateien gefunden (Standardwerte werden verwendet)", "設定ファイルが見つかりません（既定値を使用）", "구성 파일을 찾을 수 없음(기본값 사용)", "Файлы конфигурации не найдены (используются значения по умолчанию)")
	add(KeyDoctorConfigValid, "%s valid", "%s 有效", "%s gültig", "%s は有効", "%s 유효", "%s действителен")
	add(KeyDoctorDiskFree, "%.1f GB free", "剩余 %.1f GB", "%.1f GB frei", "空き %.1f GB", "여유 공간 %.1f GB", "Свободно %.1f ГБ")
	add(KeyDoctorDiskLow, "%.1f GB free - low disk space!", "剩余 %.1f GB - 磁盘空间不足！", "%.1f GB frei - wenig Speicherplatz!", "空き %.1f GB - ディスク容量が少なくなっています！", "여유 공간 %.1f GB - 디스크 공간 부족!", "Свободно %.1f ГБ — мало места на диске!")
	add(KeyDoctorDiskStatError, "could not stat %s: %v", "%s 状态读取失败：%v", "%s konnte nicht geprüft werden: %v", "%s を確認できません: %v", "%s 상태를 확인할 수 없음: %v", "не удалось проверить %s: %v")
}
