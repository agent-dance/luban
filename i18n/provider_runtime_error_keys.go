package i18n

const (
	KeyProviderUnconfigured                Key = "provider.runtime.unconfigured"
	KeyProviderUnconfiguredAction          Key = "provider.runtime.unconfigured_action"
	KeyProviderDisconnected                Key = "provider.runtime.disconnected"
	KeyProviderDisconnectedAction          Key = "provider.runtime.disconnected_action"
	KeyProviderThinkingUnsupported         Key = "provider.runtime.thinking_unsupported"
	KeyProviderCustomToolsUnsupported      Key = "provider.runtime.custom_tools_unsupported"
	KeyProviderCustomToolDefinitionInvalid Key = "provider.runtime.custom_tool_definition_invalid"
	KeyProviderRetryExceededWithoutCause   Key = "provider.runtime.retry_exceeded_without_cause"
	KeyProviderRetryExceededWithCause      Key = "provider.runtime.retry_exceeded_with_cause"
	KeyProviderUnknown                     Key = "provider.runtime.unknown"
	KeyProviderBedrockInvalidBaseURL       Key = "provider.runtime.bedrock.invalid_base_url"
	KeyProviderVertexProjectRequired       Key = "provider.runtime.vertex.project_required"
	KeyProviderVertexAPIKeyRequired        Key = "provider.runtime.vertex.api_key_required"
	KeyProviderVertexBaseURLRequired       Key = "provider.runtime.vertex.base_url_required"
	KeyProviderVertexEndpointInvalid       Key = "provider.runtime.vertex.endpoint_invalid"
	KeyCredentialHomeFailed                Key = "provider.credential.home_failed"
	KeyCredentialReadFailed                Key = "provider.credential.read_failed"
	KeyCredentialDecodeFailed              Key = "provider.credential.decode_failed"
	KeyCredentialDirectoryFailed           Key = "provider.credential.directory_failed"
	KeyCredentialEncodeFailed              Key = "provider.credential.encode_failed"
	KeyCredentialTempCreateFailed          Key = "provider.credential.temp_create_failed"
	KeyCredentialTempWriteFailed           Key = "provider.credential.temp_write_failed"
	KeyCredentialPermissionsFailed         Key = "provider.credential.permissions_failed"
	KeyCredentialTempCloseFailed           Key = "provider.credential.temp_close_failed"
	KeyCredentialReplaceFailed             Key = "provider.credential.replace_failed"
	KeyProviderDebugOpenFailed             Key = "provider.debug.open_failed"
	KeyProviderDebugWriting                Key = "provider.debug.writing"
	KeyProviderBedrockConfigInvalid        Key = "provider.runtime.bedrock.config_invalid"
	KeyProviderBedrockAWSConfigFailed      Key = "provider.runtime.bedrock.aws_config_failed"
	KeyProviderRequestEncodeFailed         Key = "provider.runtime.request_encode_failed"
	KeyProviderRequestBuildFailed          Key = "provider.runtime.request_build_failed"
	KeyProviderRequestFailed               Key = "provider.runtime.request_failed"
	KeyProviderStreamCreateFailed          Key = "provider.runtime.stream_create_failed"
	KeyProviderAnthropicUnavailable        Key = "provider.runtime.anthropic.unavailable"
	KeyProviderTokenCountInvalid           Key = "provider.runtime.anthropic.token_count_invalid"
	KeyProviderToolsConvertFailed          Key = "provider.runtime.tools_convert_failed"
	KeyProviderServerToolsConvertFailed    Key = "provider.runtime.server_tools_convert_failed"
	KeyProviderToolSchemaEncodeFailed      Key = "provider.runtime.tool_schema_encode_failed"
	KeyProviderToolSchemaDecodeFailed      Key = "provider.runtime.tool_schema_decode_failed"
	KeyProviderServerToolNameInvalid       Key = "provider.runtime.server_tool.name_invalid"
	KeyProviderServerToolDomainsConflict   Key = "provider.runtime.server_tool.domains_conflict"
	KeyProviderServerToolMaxUsesInvalid    Key = "provider.runtime.server_tool.max_uses_invalid"
	KeyProviderServerToolTypeUnsupported   Key = "provider.runtime.server_tool.type_unsupported"
	KeyProviderDebugPathEmpty              Key = "provider.debug.path_empty"
	KeyProviderDebugFileOpenFailed         Key = "provider.debug.file_open_failed"
	KeyProviderDebugPermissionsFailed      Key = "provider.debug.permissions_failed"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyProviderUnconfigured,
		"Provider %s is not configured", "Provider %s 尚未配置", "Provider %s ist nicht konfiguriert", "Provider %s は設定されていません", "Provider %s이(가) 구성되지 않았습니다", "Provider %s не настроен")
	add(KeyProviderUnconfiguredAction,
		"%s. Set %s or run /config set apiKey <key>, then restart %s", "%s。请设置 %s，或运行 /config set apiKey <key>，然后重启 %s", "%s. Setze %s oder führe /config set apiKey <key> aus und starte anschließend %s neu", "%s。%s を設定するか /config set apiKey <key> を実行し、%s を再起動してください", "%s. %s을(를) 설정하거나 /config set apiKey <key>를 실행한 다음 %s을(를) 다시 시작하세요", "%s. Задайте %s или выполните /config set apiKey <key>, затем перезапустите %s")
	add(KeyProviderDisconnected,
		"No model Provider is connected", "尚未连接模型 Provider", "Es ist kein Modell-Provider verbunden", "モデル Provider に接続されていません", "연결된 모델 Provider가 없습니다", "Нет подключённого Provider модели")
	add(KeyProviderDisconnectedAction,
		"%s. Use /connect %s --oauth or /connect %s first, then /model %s/<model>", "%s。请先运行 /connect %s --oauth 或 /connect %s，再运行 /model %s/<model>", "%s. Führe zuerst /connect %s --oauth oder /connect %s und danach /model %s/<model> aus", "%s。先に /connect %s --oauth または /connect %s を実行し、その後 /model %s/<model> を実行してください", "%s. 먼저 /connect %s --oauth 또는 /connect %s을(를) 실행한 다음 /model %s/<model>을 실행하세요", "%s. Сначала выполните /connect %s --oauth или /connect %s, затем /model %s/<model>")
	add(KeyProviderThinkingUnsupported,
		"Provider %q (model %q) does not support extended thinking; disable thinking or switch to a Provider that supports it", "Provider %q（模型 %q）不支持 extended thinking；请关闭 thinking，或切换到支持该功能的 Provider", "Provider %q (Modell %q) unterstützt Extended Thinking nicht; deaktiviere Thinking oder wechsle zu einem Provider, der es unterstützt", "Provider %q（モデル %q）は extended thinking に対応していません。thinking を無効にするか、対応する Provider に切り替えてください", "Provider %q(모델 %q)은 extended thinking을 지원하지 않습니다. thinking을 끄거나 이를 지원하는 Provider로 전환하세요", "Provider %q (модель %q) не поддерживает extended thinking; отключите thinking или выберите Provider с такой поддержкой")
	add(KeyProviderCustomToolsUnsupported,
		"Provider %q (model %q) does not explicitly support Responses custom tools; disable the custom tool experiment or use the verified public Responses endpoint with GPT-5.6",
		"Provider %q（模型 %q）未明确支持 Responses custom 工具；请关闭 custom 工具实验，或使用已验证的 GPT-5.6 公开 Responses endpoint",
		"Provider %q (Modell %q) unterstützt Responses-Custom-Tools nicht ausdrücklich; deaktiviere das Custom-Tool-Experiment oder verwende den verifizierten öffentlichen Responses-Endpunkt mit GPT-5.6",
		"Provider %q（モデル %q）は Responses custom ツールを明示的にサポートしていません。custom ツール実験を無効にするか、検証済みの GPT-5.6 公開 Responses endpoint を使用してください",
		"Provider %q(모델 %q)은 Responses custom 도구를 명시적으로 지원하지 않습니다. custom 도구 실험을 끄거나 검증된 GPT-5.6 공개 Responses endpoint를 사용하세요",
		"Provider %q (модель %q) не заявляет явную поддержку custom-инструментов Responses; отключите эксперимент или используйте проверенный публичный endpoint Responses с GPT-5.6")
	add(KeyProviderCustomToolDefinitionInvalid,
		"Custom tool %q has an invalid or unsupported freeform grammar definition",
		"Custom 工具 %q 的自由格式 grammar 定义无效或不受支持",
		"Custom-Tool %q hat eine ungültige oder nicht unterstützte Freeform-Grammatikdefinition",
		"Custom ツール %q の自由形式 grammar 定義が無効または未対応です",
		"Custom 도구 %q의 자유 형식 grammar 정의가 올바르지 않거나 지원되지 않습니다",
		"Custom-инструмент %q содержит недопустимое или неподдерживаемое определение grammar для свободного ввода")
	add(KeyProviderRetryExceededWithoutCause,
		"Retry limit reached (retry attempts: %d)", "已达到重试上限（实际重试：%d 次）", "Wiederholungslimit erreicht (Wiederholungsversuche: %d)", "再試行上限に達しました（再試行回数：%d）", "재시도 한도에 도달했습니다(재시도 횟수: %d)", "Достигнут предел повторов (число повторных попыток: %d)")
	add(KeyProviderRetryExceededWithCause,
		"Retry limit reached (retry attempts: %d): %v", "已达到重试上限（实际重试：%d 次）：%v", "Wiederholungslimit erreicht (Wiederholungsversuche: %d): %v", "再試行上限に達しました（再試行回数：%d）：%v", "재시도 한도에 도달했습니다(재시도 횟수: %d): %v", "Достигнут предел повторов (число повторных попыток: %d): %v")
	add(KeyProviderUnknown,
		"Unknown Provider %q; choose one of: %s", "未知 Provider %q；请选择以下选项之一：%s", "Unbekannter Provider %q; wähle einen der folgenden: %s", "不明な Provider %q です。次のいずれかを選択してください: %s", "알 수 없는 Provider %q입니다. 다음 중 하나를 선택하세요: %s", "Неизвестный Provider %q; выберите один из следующих: %s")
	add(KeyProviderBedrockInvalidBaseURL,
		"Base URL must use https:// (or http://localhost for testing): %q", "Base URL 必须使用 https://（测试时可使用 http://localhost）：%q", "Die Base URL muss https:// verwenden (oder http://localhost für Tests): %q", "Base URL には https:// を使用してください（テストでは http://localhost も可）: %q", "Base URL은 https://를 사용해야 합니다(테스트 시 http://localhost 허용): %q", "Base URL должен использовать https:// (либо http://localhost для тестов): %q")
	add(KeyProviderVertexProjectRequired,
		"GOOGLE_CLOUD_PROJECT (or ANTHROPIC_VERTEX_PROJECT_ID) must be set", "必须设置 GOOGLE_CLOUD_PROJECT（或 ANTHROPIC_VERTEX_PROJECT_ID）", "GOOGLE_CLOUD_PROJECT (oder ANTHROPIC_VERTEX_PROJECT_ID) muss gesetzt sein", "GOOGLE_CLOUD_PROJECT（または ANTHROPIC_VERTEX_PROJECT_ID）を設定してください", "GOOGLE_CLOUD_PROJECT(또는 ANTHROPIC_VERTEX_PROJECT_ID)를 설정해야 합니다", "Необходимо задать GOOGLE_CLOUD_PROJECT (или ANTHROPIC_VERTEX_PROJECT_ID)")
	add(KeyProviderVertexAPIKeyRequired,
		"The Vertex custom endpoint requires an API key", "Vertex 自定义 endpoint 需要 API key", "Der benutzerdefinierte Vertex-Endpunkt benötigt einen API-Schlüssel", "Vertex カスタム endpoint には API キーが必要です", "Vertex 사용자 지정 endpoint에는 API 키가 필요합니다", "Для пользовательского endpoint Vertex требуется API-ключ")
	add(KeyProviderVertexBaseURLRequired,
		"The Vertex custom endpoint requires a Base URL", "Vertex 自定义 endpoint 需要 Base URL", "Der benutzerdefinierte Vertex-Endpunkt benötigt eine Base URL", "Vertex カスタム endpoint には Base URL が必要です", "Vertex 사용자 지정 endpoint에는 Base URL이 필요합니다", "Для пользовательского endpoint Vertex требуется Base URL")
	add(KeyProviderVertexEndpointInvalid,
		"Invalid Vertex custom endpoint", "Vertex 自定义 endpoint 无效", "Ungültiger benutzerdefinierter Vertex-Endpunkt", "Vertex カスタム endpoint が無効です", "Vertex 사용자 지정 endpoint가 올바르지 않습니다", "Недопустимый пользовательский endpoint Vertex")
	add(KeyCredentialHomeFailed,
		"Could not determine the home directory", "无法确定用户主目录", "Das Home-Verzeichnis konnte nicht ermittelt werden", "ホームディレクトリを特定できませんでした", "홈 디렉터리를 확인할 수 없습니다", "Не удалось определить домашний каталог")
	add(KeyCredentialReadFailed,
		"Could not read credential store %s", "无法读取凭据存储 %s", "Zugangsdaten-Speicher %s konnte nicht gelesen werden", "認証情報ストア %s を読み込めませんでした", "자격 증명 저장소 %s을(를) 읽을 수 없습니다", "Не удалось прочитать хранилище учётных данных %s")
	add(KeyCredentialDecodeFailed,
		"Could not decode credential store %s", "无法解析凭据存储 %s", "Zugangsdaten-Speicher %s konnte nicht dekodiert werden", "認証情報ストア %s をデコードできませんでした", "자격 증명 저장소 %s을(를) 디코딩할 수 없습니다", "Не удалось декодировать хранилище учётных данных %s")
	add(KeyCredentialDirectoryFailed,
		"Could not create credential directory %s", "无法创建凭据目录 %s", "Zugangsdaten-Verzeichnis %s konnte nicht erstellt werden", "認証情報ディレクトリ %s を作成できませんでした", "자격 증명 디렉터리 %s을(를) 만들 수 없습니다", "Не удалось создать каталог учётных данных %s")
	add(KeyCredentialEncodeFailed,
		"Could not encode credentials", "无法编码凭据", "Zugangsdaten konnten nicht kodiert werden", "認証情報をエンコードできませんでした", "자격 증명을 인코딩할 수 없습니다", "Не удалось закодировать учётные данные")
	add(KeyCredentialTempCreateFailed,
		"Could not create the temporary credential file", "无法创建临时凭据文件", "Temporäre Zugangsdaten-Datei konnte nicht erstellt werden", "一時認証情報ファイルを作成できませんでした", "임시 자격 증명 파일을 만들 수 없습니다", "Не удалось создать временный файл учётных данных")
	add(KeyCredentialTempWriteFailed,
		"Could not write the temporary credential file", "无法写入临时凭据文件", "Temporäre Zugangsdaten-Datei konnte nicht geschrieben werden", "一時認証情報ファイルに書き込めませんでした", "임시 자격 증명 파일에 쓸 수 없습니다", "Не удалось записать временный файл учётных данных")
	add(KeyCredentialPermissionsFailed,
		"Could not secure the credential file permissions", "无法设置凭据文件的安全权限", "Berechtigungen der Zugangsdaten-Datei konnten nicht abgesichert werden", "認証情報ファイルの権限を安全に設定できませんでした", "자격 증명 파일 권한을 안전하게 설정할 수 없습니다", "Не удалось установить безопасные права файла учётных данных")
	add(KeyCredentialTempCloseFailed,
		"Could not close the temporary credential file", "无法关闭临时凭据文件", "Temporäre Zugangsdaten-Datei konnte nicht geschlossen werden", "一時認証情報ファイルを閉じられませんでした", "임시 자격 증명 파일을 닫을 수 없습니다", "Не удалось закрыть временный файл учётных данных")
	add(KeyCredentialReplaceFailed,
		"Could not replace the credential store", "无法替换凭据存储", "Zugangsdaten-Speicher konnte nicht ersetzt werden", "認証情報ストアを置き換えられませんでした", "자격 증명 저장소를 교체할 수 없습니다", "Не удалось заменить хранилище учётных данных")
	add(KeyProviderDebugOpenFailed,
		"[anthropic-debug] Could not open debug log %q: %v", "[anthropic-debug] 无法打开 debug log %q：%v", "[anthropic-debug] Debug-Log %q konnte nicht geöffnet werden: %v", "[anthropic-debug] debug log %q を開けませんでした: %v", "[anthropic-debug] debug log %q을(를) 열 수 없습니다: %v", "[anthropic-debug] Не удалось открыть debug log %q: %v")
	add(KeyProviderDebugWriting,
		"[anthropic-debug] Writing raw SSE debug log to %s", "[anthropic-debug] 正在将原始 SSE debug log 写入 %s", "[anthropic-debug] Raw-SSE-Debug-Log wird nach %s geschrieben", "[anthropic-debug] raw SSE debug log を %s に書き込みます", "[anthropic-debug] raw SSE debug log를 %s에 기록합니다", "[anthropic-debug] Raw SSE debug log записывается в %s")
	add(KeyProviderBedrockConfigInvalid, "Invalid Bedrock configuration: %v", "Bedrock 配置无效：%v", "Ungültige Bedrock-Konfiguration: %v", "Bedrock 設定が無効です: %v", "잘못된 Bedrock 구성입니다: %v", "Недопустимая конфигурация Bedrock: %v")
	add(KeyProviderBedrockAWSConfigFailed, "Could not build the AWS configuration for Bedrock: %v", "无法为 Bedrock 构建 AWS 配置：%v", "AWS-Konfiguration für Bedrock konnte nicht erstellt werden: %v", "Bedrock 用の AWS 設定を作成できませんでした: %v", "Bedrock용 AWS 구성을 만들 수 없습니다: %v", "Не удалось сформировать конфигурацию AWS для Bedrock: %v")
	add(KeyProviderRequestEncodeFailed, "Could not encode the Provider request: %v", "无法编码 Provider 请求：%v", "Provider-Anfrage konnte nicht kodiert werden: %v", "Provider リクエストをエンコードできませんでした: %v", "Provider 요청을 인코딩할 수 없습니다: %v", "Не удалось закодировать запрос Provider: %v")
	add(KeyProviderRequestBuildFailed, "Could not build the Provider request: %v", "无法构建 Provider 请求：%v", "Provider-Anfrage konnte nicht erstellt werden: %v", "Provider リクエストを作成できませんでした: %v", "Provider 요청을 만들 수 없습니다: %v", "Не удалось сформировать запрос Provider: %v")
	add(KeyProviderRequestFailed, "%s request failed: %v", "%s 请求失败：%v", "%s-Anfrage fehlgeschlagen: %v", "%s リクエストに失敗しました: %v", "%s 요청에 실패했습니다: %v", "Запрос %s завершился ошибкой: %v")
	add(KeyProviderStreamCreateFailed, "Could not create the response stream: %v", "无法创建响应 stream：%v", "Antwort-Stream konnte nicht erstellt werden: %v", "応答 stream を作成できませんでした: %v", "응답 stream을 만들 수 없습니다: %v", "Не удалось создать stream ответа: %v")
	add(KeyProviderAnthropicUnavailable, "The Anthropic Provider is unavailable", "Anthropic Provider 不可用", "Der Anthropic Provider ist nicht verfügbar", "Anthropic Provider を利用できません", "Anthropic Provider를 사용할 수 없습니다", "Anthropic Provider недоступен")
	add(KeyProviderTokenCountInvalid, "The Anthropic token-count response was invalid", "Anthropic token count 响应无效", "Die Anthropic-Antwort zur Token-Zählung war ungültig", "Anthropic の token count 応答が無効です", "Anthropic token count 응답이 올바르지 않습니다", "Недопустимый ответ Anthropic с числом token")
	add(KeyProviderToolsConvertFailed, "Could not convert tool definitions: %v", "无法转换工具定义：%v", "Tool-Definitionen konnten nicht konvertiert werden: %v", "ツール定義を変換できませんでした: %v", "도구 정의를 변환할 수 없습니다: %v", "Не удалось преобразовать определения инструментов: %v")
	add(KeyProviderServerToolsConvertFailed, "Could not convert server tool definitions: %v", "无法转换 server 工具定义：%v", "Server-Tool-Definitionen konnten nicht konvertiert werden: %v", "server ツール定義を変換できませんでした: %v", "server 도구 정의를 변환할 수 없습니다: %v", "Не удалось преобразовать определения инструментов server: %v")
	add(KeyProviderToolSchemaEncodeFailed, "Could not encode the schema for tool %q: %v", "无法编码工具 %q 的 schema：%v", "Schema für Tool %q konnte nicht kodiert werden: %v", "ツール %q の schema をエンコードできませんでした: %v", "도구 %q의 schema를 인코딩할 수 없습니다: %v", "Не удалось закодировать schema инструмента %q: %v")
	add(KeyProviderToolSchemaDecodeFailed, "Could not decode the schema for tool %q: %v", "无法解码工具 %q 的 schema：%v", "Schema für Tool %q konnte nicht dekodiert werden: %v", "ツール %q の schema をデコードできませんでした: %v", "도구 %q의 schema를 디코딩할 수 없습니다: %v", "Не удалось декодировать schema инструмента %q: %v")
	add(KeyProviderServerToolNameInvalid, "%s server tool name must be web_search", "%s server 工具名称必须是 web_search", "Der Name des Server-Tools %s muss web_search sein", "%s server ツール名は web_search にしてください", "%s server 도구 이름은 web_search여야 합니다", "Имя инструмента server %s должно быть web_search")
	add(KeyProviderServerToolDomainsConflict, "%s cannot specify both allowed_domains and blocked_domains", "%s 不能同时指定 allowed_domains 和 blocked_domains", "%s darf nicht gleichzeitig allowed_domains und blocked_domains angeben", "%s では allowed_domains と blocked_domains を同時に指定できません", "%s은(는) allowed_domains와 blocked_domains를 동시에 지정할 수 없습니다", "%s не может одновременно задавать allowed_domains и blocked_domains")
	add(KeyProviderServerToolMaxUsesInvalid, "%s max_uses cannot be negative", "%s 的 max_uses 不能为负数", "max_uses von %s darf nicht negativ sein", "%s の max_uses は負数にできません", "%s의 max_uses는 음수일 수 없습니다", "max_uses для %s не может быть отрицательным")
	add(KeyProviderServerToolTypeUnsupported, "Unsupported Anthropic server tool type %q", "不支持的 Anthropic server 工具类型 %q", "Nicht unterstützter Anthropic-Server-Tool-Typ %q", "サポートされていない Anthropic server ツールタイプ %q", "지원되지 않는 Anthropic server 도구 유형 %q", "Неподдерживаемый тип инструмента Anthropic server %q")
	add(KeyProviderDebugPathEmpty, "The debug file path is empty", "debug 文件路径为空", "Der Pfad zur Debug-Datei ist leer", "debug ファイルのパスが空です", "debug 파일 경로가 비어 있습니다", "Путь к debug-файлу пуст")
	add(KeyProviderDebugFileOpenFailed, "Could not open debug file %q: %v", "无法打开 debug 文件 %q：%v", "Debug-Datei %q konnte nicht geöffnet werden: %v", "debug ファイル %q を開けませんでした: %v", "debug 파일 %q을(를) 열 수 없습니다: %v", "Не удалось открыть debug-файл %q: %v")
	add(KeyProviderDebugPermissionsFailed, "Could not restrict permissions for debug file %q: %v", "无法限制 debug 文件 %q 的权限：%v", "Berechtigungen für Debug-Datei %q konnten nicht eingeschränkt werden: %v", "debug ファイル %q の権限を制限できませんでした: %v", "debug 파일 %q의 권한을 제한할 수 없습니다: %v", "Не удалось ограничить права debug-файла %q: %v")
}
