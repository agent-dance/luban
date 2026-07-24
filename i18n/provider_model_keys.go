package i18n

// Semantic copy for Provider connection state and the model/reasoning picker.
const (
	KeyProviderConnectionUnknown             Key = "provider.connection.unknown"
	KeyProviderConnectionChooseRegistered    Key = "provider.connection.choose_registered"
	KeyProviderConnectionNotConnected        Key = "provider.connection.not_connected"
	KeyProviderConnectionConnectedEnv        Key = "provider.connection.connected_env"
	KeyProviderConnectionEnvNotSet           Key = "provider.connection.env_not_set"
	KeyProviderConnectionEnvSet              Key = "provider.connection.env_set"
	KeyProviderConnectionLocalUnchecked      Key = "provider.connection.local_unchecked"
	KeyProviderConnectionCredentialAPIKey    Key = "provider.connection.credential_api_key"
	KeyProviderConnectionImportedEnv         Key = "provider.connection.imported_env"
	KeyProviderConnectionOAuth               Key = "provider.connection.oauth"
	KeyProviderConnectionOAuthRefresh        Key = "provider.connection.oauth_refresh"
	KeyProviderConnectionChatGPTOAuth        Key = "provider.connection.chatgpt_oauth"
	KeyProviderConnectionChatGPTOAuthRefresh Key = "provider.connection.chatgpt_oauth_refresh"
	KeyProviderConnectionAWSBearer           Key = "provider.connection.aws_bearer"
	KeyProviderConnectionAWSAccessKey        Key = "provider.connection.aws_access_key"
	KeyProviderConnectionAWSProfile          Key = "provider.connection.aws_profile"
	KeyProviderConnectionAWSMissing          Key = "provider.connection.aws_missing"
	KeyProviderConnectionGCPApplication      Key = "provider.connection.gcp_application"
	KeyProviderConnectionGCPDefault          Key = "provider.connection.gcp_default"
	KeyProviderConnectionGCPProjectMissing   Key = "provider.connection.gcp_project_missing"
	KeyProviderConnectionGCPADCMissing       Key = "provider.connection.gcp_adc_missing"
	KeyProviderConnectionGCPMissing          Key = "provider.connection.gcp_missing"
	KeyProviderSetupBedrock                  Key = "provider.setup.bedrock"
	KeyProviderSetupVertex                   Key = "provider.setup.vertex"
	KeyProviderSetupOllama                   Key = "provider.setup.ollama"
	KeyProviderSetupEnvOrConnect             Key = "provider.setup.env_or_connect"
	KeyProviderSetupGeneric                  Key = "provider.setup.generic"
	KeyProviderPickerActionsDefault          Key = "provider.picker.actions.default"
	KeyProviderPickerActionsConnected        Key = "provider.picker.actions.connected"
	KeyProviderPickerActionsConfigure        Key = "provider.picker.actions.configure"
	KeyProviderPickerModelCount              Key = "provider.picker.model_count"
	KeyProviderConnectTitle                  Key = "provider.connect.title"
	KeyProviderReconnectTitle                Key = "provider.reconnect.title"
	KeyProviderAuthEnvHint                   Key = "provider.auth.env_hint"
	KeyProviderAuthAPIKeyLabel               Key = "provider.auth.api_key.label"
	KeyProviderAuthAPIKeyDescription         Key = "provider.auth.api_key.description"
	KeyProviderAuthOAuthLabel                Key = "provider.auth.oauth.label"
	KeyProviderAuthOAuthDescription          Key = "provider.auth.oauth.description"
	KeyProviderAuthDeviceLabel               Key = "provider.auth.device.label"
	KeyProviderAuthDeviceDescription         Key = "provider.auth.device.description"
	KeyProviderAuthAWSLabel                  Key = "provider.auth.aws.label"
	KeyProviderAuthAWSDescription            Key = "provider.auth.aws.description"
	KeyProviderAuthGCPLabel                  Key = "provider.auth.gcp.label"
	KeyProviderAuthGCPDescription            Key = "provider.auth.gcp.description"
	KeyProviderConnectExternalHint           Key = "provider.connect.external_hint"
	KeyProviderConnectSelectMethod           Key = "provider.connect.select_method"
	KeyProviderConnectInputHint              Key = "provider.connect.input_hint"
	KeyProviderConnectDefaultEndpoint        Key = "provider.connect.default_endpoint"
	KeyProviderConnectBaseURL                Key = "provider.connect.base_url"
	KeyProviderConnectRequired               Key = "provider.connect.required"
	KeyProviderConnectAPIKey                 Key = "provider.connect.api_key"
	KeyModelTagVision                        Key = "model.tag.vision"
	KeyModelTagText                          Key = "model.tag.text"
	KeyModelTagEffort                        Key = "model.tag.effort"
	KeyModelTagThinking                      Key = "model.tag.thinking"
	KeyReasoningLabelLow                     Key = "reasoning.label.low"
	KeyReasoningLabelMedium                  Key = "reasoning.label.medium"
	KeyReasoningLabelHigh                    Key = "reasoning.label.high"
	KeyReasoningLabelExtraHigh               Key = "reasoning.label.extra_high"
	KeyReasoningLabelMax                     Key = "reasoning.label.max"
	KeyReasoningLabelDefault                 Key = "reasoning.label.default"
	KeyReasoningDescriptionLow               Key = "reasoning.description.low"
	KeyReasoningDescriptionMedium            Key = "reasoning.description.medium"
	KeyReasoningDescriptionHigh              Key = "reasoning.description.high"
	KeyReasoningDescriptionExtraHigh         Key = "reasoning.description.extra_high"
	KeyReasoningDescriptionMax               Key = "reasoning.description.max"
	KeyReasoningDescriptionProviderDefined   Key = "reasoning.description.provider_defined"
	KeyModelDescriptionGPT55                 Key = "model.description.gpt_5_5"
	KeyModelDescriptionGPT54                 Key = "model.description.gpt_5_4"
	KeyModelDescriptionGPT54Mini             Key = "model.description.gpt_5_4_mini"
	KeyModelDescriptionGPT54Nano             Key = "model.description.gpt_5_4_nano"
	KeyModelDescriptionGPT53Codex            Key = "model.description.gpt_5_3_codex"
	KeyModelDescriptionGPT52                 Key = "model.description.gpt_5_2"
	KeyModelDescriptionGPT5                  Key = "model.description.gpt_5"
	KeyModelDescriptionGPT5Mini              Key = "model.description.gpt_5_mini"
	KeyModelDescriptionGPT4O                 Key = "model.description.gpt_4o"
	KeyModelDescriptionGPT4OMini             Key = "model.description.gpt_4o_mini"
	KeyModelDescriptionCodexSpark            Key = "model.description.codex_spark"
	KeyModelDescriptionCodex                 Key = "model.description.codex"
	KeyModelDescriptionDeepSeekPro           Key = "model.description.deepseek_pro"
	KeyModelDescriptionDeepSeekFlash         Key = "model.description.deepseek_flash"
	KeyModelDescriptionClaudeSonnet          Key = "model.description.claude_sonnet"
	KeyModelDescriptionClaudeOpus            Key = "model.description.claude_opus"
	KeyModelDescriptionClaudeHaiku           Key = "model.description.claude_haiku"
	KeyModelDescriptionGeminiPro             Key = "model.description.gemini_pro"
	KeyModelDescriptionGeminiFlashLite       Key = "model.description.gemini_flash_lite"
	KeyModelDescriptionGeminiFlash           Key = "model.description.gemini_flash"
	KeyModelDescriptionReasoning             Key = "model.description.reasoning"
	KeyModelDescriptionMultimodal            Key = "model.description.multimodal"
	KeyModelDescriptionGeneral               Key = "model.description.general"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyProviderConnectionUnknown, "Unknown provider", "未知 Provider", "Unbekannter Provider", "不明な Provider", "알 수 없는 Provider", "Неизвестный Provider")
	add(KeyProviderConnectionChooseRegistered, "Choose one of the registered providers.", "请选择已注册的 Provider。", "Wähle einen registrierten Provider aus.", "登録済みの Provider を選択してください。", "등록된 Provider 중 하나를 선택하세요.", "Выберите зарегистрированный Provider.")
	add(KeyProviderConnectionNotConnected, "Not connected", "未连接", "Nicht verbunden", "未接続", "연결되지 않음", "Не подключён")
	add(KeyProviderConnectionConnectedEnv, "Connected (env: %s)", "已连接（环境变量：%s）", "Verbunden (Umgebungsvariable: %s)", "接続済み（環境変数：%s）", "연결됨(환경 변수: %s)", "Подключён (переменная среды: %s)")
	add(KeyProviderConnectionEnvNotSet, "Not connected (%s not set)", "未连接（未设置 %s）", "Nicht verbunden (%s ist nicht gesetzt)", "未接続（%s が未設定）", "연결되지 않음(%s이(가) 설정되지 않음)", "Не подключён (%s не задана)")
	add(KeyProviderConnectionEnvSet, "Connected (%s set)", "已连接（已设置 %s）", "Verbunden (%s ist gesetzt)", "接続済み（%s を設定済み）", "연결됨(%s 설정됨)", "Подключён (%s задана)")
	add(KeyProviderConnectionLocalUnchecked, "Local provider (no API key required; server not checked)", "本地 Provider（无需 API key；尚未检查服务器）", "Lokaler Provider (kein API-Schlüssel erforderlich; Server nicht geprüft)", "ローカル Provider（API キー不要、サーバー未確認）", "로컬 Provider(API 키 불필요, 서버 확인 안 됨)", "Локальный Provider (API-ключ не требуется; сервер не проверен)")
	add(KeyProviderConnectionCredentialAPIKey, "Connected (credential store API key)", "已连接（凭据存储中的 API key）", "Verbunden (API-Schlüssel im Zugangsdaten-Speicher)", "接続済み（認証情報ストアの API キー）", "연결됨(자격 증명 저장소의 API 키)", "Подключён (API-ключ в хранилище учётных данных)")
	add(KeyProviderConnectionImportedEnv, "Connected (imported env credential)", "已连接（已导入环境凭据）", "Verbunden (importierte Umgebungs-Zugangsdaten)", "接続済み（インポートした環境認証情報）", "연결됨(가져온 환경 자격 증명)", "Подключён (импортированные данные среды)")
	add(KeyProviderConnectionOAuth, "Connected (OAuth)", "已连接（OAuth）", "Verbunden (OAuth)", "接続済み（OAuth）", "연결됨(OAuth)", "Подключён (OAuth)")
	add(KeyProviderConnectionOAuthRefresh, "Connected (OAuth refresh token)", "已连接（OAuth refresh token）", "Verbunden (OAuth-Refresh-Token)", "接続済み（OAuth refresh token）", "연결됨(OAuth refresh token)", "Подключён (refresh token OAuth)")
	add(KeyProviderConnectionChatGPTOAuth, "Connected (ChatGPT OAuth)", "已连接（ChatGPT OAuth）", "Verbunden (ChatGPT OAuth)", "接続済み（ChatGPT OAuth）", "연결됨(ChatGPT OAuth)", "Подключён (ChatGPT OAuth)")
	add(KeyProviderConnectionChatGPTOAuthRefresh, "Connected (ChatGPT OAuth refresh token)", "已连接（ChatGPT OAuth refresh token）", "Verbunden (ChatGPT-OAuth-Refresh-Token)", "接続済み（ChatGPT OAuth refresh token）", "연결됨(ChatGPT OAuth refresh token)", "Подключён (refresh token ChatGPT OAuth)")
	add(KeyProviderConnectionAWSBearer, "Connected (AWS bearer token)", "已连接（AWS bearer token）", "Verbunden (AWS-Bearer-Token)", "接続済み（AWS bearer token）", "연결됨(AWS bearer token)", "Подключён (bearer token AWS)")
	add(KeyProviderConnectionAWSAccessKey, "Connected (AWS access key)", "已连接（AWS access key）", "Verbunden (AWS-Zugriffsschlüssel)", "接続済み（AWS access key）", "연결됨(AWS access key)", "Подключён (access key AWS)")
	add(KeyProviderConnectionAWSProfile, "Connected (AWS profile)", "已连接（AWS profile）", "Verbunden (AWS-Profil)", "接続済み（AWS profile）", "연결됨(AWS profile)", "Подключён (профиль AWS)")
	add(KeyProviderConnectionAWSMissing, "Not connected (AWS credentials not configured)", "未连接（未配置 AWS 凭据）", "Nicht verbunden (AWS-Zugangsdaten sind nicht konfiguriert)", "未接続（AWS 認証情報が未設定）", "연결되지 않음(AWS 자격 증명 미구성)", "Не подключён (учётные данные AWS не настроены)")
	add(KeyProviderConnectionGCPApplication, "Connected (GCP application credentials and project)", "已连接（GCP application credentials 和 project）", "Verbunden (GCP-Anwendungszugangsdaten und Projekt)", "接続済み（GCP application credentials と project）", "연결됨(GCP application credentials 및 project)", "Подключён (application credentials и проект GCP)")
	add(KeyProviderConnectionGCPDefault, "Connected (GCP application-default credentials and project)", "已连接（GCP application-default credentials 和 project）", "Verbunden (GCP Application Default Credentials und Projekt)", "接続済み（GCP application-default credentials と project）", "연결됨(GCP application-default credentials 및 project)", "Подключён (Application Default Credentials и проект GCP)")
	add(KeyProviderConnectionGCPProjectMissing, "Not connected (GCP project not configured)", "未连接（未配置 GCP project）", "Nicht verbunden (GCP-Projekt ist nicht konfiguriert)", "未接続（GCP project が未設定）", "연결되지 않음(GCP project 미구성)", "Не подключён (проект GCP не настроен)")
	add(KeyProviderConnectionGCPADCMissing, "Not connected (GCP project set, ADC credentials not detected)", "未连接（已设置 GCP project，但未检测到 ADC 凭据）", "Nicht verbunden (GCP-Projekt ist gesetzt, aber keine ADC-Zugangsdaten erkannt)", "未接続（GCP project は設定済みですが、ADC 認証情報を検出できません）", "연결되지 않음(GCP project는 설정되었지만 ADC 자격 증명이 감지되지 않음)", "Не подключён (проект GCP задан, но данные ADC не обнаружены)")
	add(KeyProviderConnectionGCPMissing, "Not connected (GCP ADC/project not configured)", "未连接（未配置 GCP ADC 或 project）", "Nicht verbunden (GCP ADC oder Projekt ist nicht konfiguriert)", "未接続（GCP ADC または project が未設定）", "연결되지 않음(GCP ADC 또는 project 미구성)", "Не подключён (GCP ADC или проект не настроены)")
	add(KeyProviderSetupBedrock, "Set AWS_PROFILE, AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY, or AWS_BEARER_TOKEN_BEDROCK, then reopen /model.", "请设置 AWS_PROFILE、AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY 或 AWS_BEARER_TOKEN_BEDROCK，然后重新打开 /model。", "Setze AWS_PROFILE, AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY oder AWS_BEARER_TOKEN_BEDROCK und öffne danach /model erneut.", "AWS_PROFILE、AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY、または AWS_BEARER_TOKEN_BEDROCK を設定し、/model を開き直してください。", "AWS_PROFILE, AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY 또는 AWS_BEARER_TOKEN_BEDROCK을 설정한 뒤 /model을 다시 여세요.", "Задайте AWS_PROFILE, AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY или AWS_BEARER_TOKEN_BEDROCK, затем снова откройте /model.")
	add(KeyProviderSetupVertex, "Set GOOGLE_APPLICATION_CREDENTIALS plus GOOGLE_CLOUD_PROJECT, or run gcloud application-default login and set a Vertex project, then reopen /model.", "请设置 GOOGLE_APPLICATION_CREDENTIALS 和 GOOGLE_CLOUD_PROJECT，或运行 gcloud application-default login 并设置 Vertex project，然后重新打开 /model。", "Setze GOOGLE_APPLICATION_CREDENTIALS und GOOGLE_CLOUD_PROJECT oder führe gcloud application-default login aus und lege ein Vertex-Projekt fest. Öffne danach /model erneut.", "GOOGLE_APPLICATION_CREDENTIALS と GOOGLE_CLOUD_PROJECT を設定するか、gcloud application-default login を実行して Vertex project を設定し、/model を開き直してください。", "GOOGLE_APPLICATION_CREDENTIALS와 GOOGLE_CLOUD_PROJECT를 설정하거나 gcloud application-default login을 실행하고 Vertex project를 설정한 뒤 /model을 다시 여세요.", "Задайте GOOGLE_APPLICATION_CREDENTIALS и GOOGLE_CLOUD_PROJECT либо выполните gcloud application-default login и укажите проект Vertex, затем снова откройте /model.")
	add(KeyProviderSetupOllama, "Start Ollama locally and make sure OLLAMA_BASE_URL points at the running server.", "请在本地启动 Ollama，并确保 OLLAMA_BASE_URL 指向正在运行的服务器。", "Starte Ollama lokal und stelle sicher, dass OLLAMA_BASE_URL auf den laufenden Server zeigt.", "Ollama をローカルで起動し、OLLAMA_BASE_URL が実行中のサーバーを指していることを確認してください。", "Ollama를 로컬에서 시작하고 OLLAMA_BASE_URL이 실행 중인 서버를 가리키는지 확인하세요.", "Запустите Ollama локально и убедитесь, что OLLAMA_BASE_URL указывает на работающий сервер.")
	add(KeyProviderSetupEnvOrConnect, "Set %s or run /connect %s.", "请设置 %s，或运行 /connect %s。", "Setze %s oder führe /connect %s aus.", "%s を設定するか、/connect %s を実行してください。", "%s을(를) 설정하거나 /connect %s를 실행하세요.", "Задайте %s или выполните /connect %s.")
	add(KeyProviderSetupGeneric, "Configure credentials for this provider, then reopen /model.", "请为此 Provider 配置凭据，然后重新打开 /model。", "Konfiguriere Zugangsdaten für diesen Provider und öffne danach /model erneut.", "この Provider の認証情報を設定し、/model を開き直してください。", "이 Provider의 자격 증명을 구성한 뒤 /model을 다시 여세요.", "Настройте учётные данные для этого Provider, затем снова откройте /model.")

	add(KeyProviderPickerActionsDefault, "↑/↓ navigate, Enter select, Esc close", "↑/↓ 移动，Enter 选择，Esc 关闭", "↑/↓ navigieren, Enter auswählen, Esc schließen", "↑/↓ 移動、Enter 選択、Esc 閉じる", "↑/↓ 이동, Enter 선택, Esc 닫기", "↑/↓ навигация, Enter выбрать, Esc закрыть")
	add(KeyProviderPickerActionsConnected, "↑/↓ navigate, Enter select, R reconnect, Esc close", "↑/↓ 移动，Enter 选择，R 重新连接，Esc 关闭", "↑/↓ navigieren, Enter auswählen, R erneut verbinden, Esc schließen", "↑/↓ 移動、Enter 選択、R 再接続、Esc 閉じる", "↑/↓ 이동, Enter 선택, R 다시 연결, Esc 닫기", "↑/↓ навигация, Enter выбрать, R переподключить, Esc закрыть")
	add(KeyProviderPickerActionsConfigure, "↑/↓ navigate, Enter select, R configure, Esc close", "↑/↓ 移动，Enter 选择，R 配置，Esc 关闭", "↑/↓ navigieren, Enter auswählen, R konfigurieren, Esc schließen", "↑/↓ 移動、Enter 選択、R 設定、Esc 閉じる", "↑/↓ 이동, Enter 선택, R 구성, Esc 닫기", "↑/↓ навигация, Enter выбрать, R настроить, Esc закрыть")
	add(KeyProviderPickerModelCount, " (%d models)", "（%d 个模型）", " (%d Modelle)", "（%d モデル）", " (모델 %d개)", " (%d моделей)")
	add(KeyProviderConnectTitle, "Connect %s — Esc to go back", "连接 %s — 按 Esc 返回", "%s verbinden — Esc zum Zurückgehen", "%s に接続 — Esc で戻る", "%s 연결 — Esc로 돌아가기", "Подключить %s — Esc для возврата")
	add(KeyProviderReconnectTitle, "Reconnect %s — Esc to go back", "重新连接 %s — 按 Esc 返回", "%s erneut verbinden — Esc zum Zurückgehen", "%s に再接続 — Esc で戻る", "%s 다시 연결 — Esc로 돌아가기", "Переподключить %s — Esc для возврата")
	add(KeyProviderAuthEnvHint, " (environment variable: %s)", "（环境变量：%s）", " (Umgebungsvariable: %s)", "（環境変数：%s）", " (환경 변수: %s)", " (переменная среды: %s)")
	add(KeyProviderAuthAPIKeyLabel, "🔑 API key + Base URL%s", "🔑 API key + Base URL%s", "🔑 API-Schlüssel + Base URL%s", "🔑 API キー + Base URL%s", "🔑 API 키 + Base URL%s", "🔑 API-ключ + Base URL%s")
	add(KeyProviderAuthAPIKeyDescription, "Use the Provider default or a compatible endpoint", "使用 Provider 默认 endpoint 或兼容 endpoint", "Provider-Standard oder kompatiblen Endpoint verwenden", "Provider のデフォルトまたは互換 endpoint を使用", "Provider 기본값 또는 호환 endpoint 사용", "Использовать endpoint Provider по умолчанию или совместимый endpoint")
	add(KeyProviderAuthOAuthLabel, "🌐 OAuth (browser)", "🌐 OAuth（浏览器）", "🌐 OAuth (Browser)", "🌐 OAuth（ブラウザー）", "🌐 OAuth(브라우저)", "🌐 OAuth (браузер)")
	add(KeyProviderAuthOAuthDescription, "Open a browser for authentication", "打开浏览器进行认证", "Browser zur Authentifizierung öffnen", "ブラウザーを開いて認証", "인증을 위해 브라우저 열기", "Открыть браузер для авторизации")
	add(KeyProviderAuthDeviceLabel, "📱 Device Authorization", "📱 Device Authorization", "📱 Device Authorization", "📱 Device Authorization", "📱 Device Authorization", "📱 Device Authorization")
	add(KeyProviderAuthDeviceDescription, "Get a code to enter in the browser", "获取需要在浏览器中输入的代码", "Code zum Eingeben im Browser abrufen", "ブラウザーに入力するコードを取得", "브라우저에 입력할 코드 받기", "Получить код для ввода в браузере")
	add(KeyProviderAuthAWSLabel, "AWS credentials", "AWS 凭据", "AWS-Zugangsdaten", "AWS 認証情報", "AWS 자격 증명", "Учётные данные AWS")
	add(KeyProviderAuthAWSDescription, "Use the AWS credential chain", "使用 AWS credential chain", "AWS-Zugangsdatenkette verwenden", "AWS credential chain を使用", "AWS credential chain 사용", "Использовать цепочку учётных данных AWS")
	add(KeyProviderAuthGCPLabel, "Google Cloud ADC", "Google Cloud ADC", "Google Cloud ADC", "Google Cloud ADC", "Google Cloud ADC", "Google Cloud ADC")
	add(KeyProviderAuthGCPDescription, "Use Application Default Credentials", "使用 Application Default Credentials", "Application Default Credentials verwenden", "Application Default Credentials を使用", "Application Default Credentials 사용", "Использовать Application Default Credentials")
	add(KeyProviderConnectExternalHint, "Configure this Provider outside the TUI, then reopen /model.", "请在 TUI 外配置此 Provider，然后重新打开 /model。", "Konfiguriere diesen Provider außerhalb der TUI und öffne danach /model erneut.", "TUI の外でこの Provider を設定し、/model を開き直してください。", "TUI 외부에서 이 Provider를 구성한 뒤 /model을 다시 여세요.", "Настройте этот Provider вне TUI, затем снова откройте /model.")
	add(KeyProviderConnectSelectMethod, "  Select an authentication method: ↑/↓ navigate, Enter select", "  选择认证方式：↑/↓ 移动，Enter 选择", "  Authentifizierungsmethode auswählen: ↑/↓ navigieren, Enter auswählen", "  認証方法を選択：↑/↓ 移動、Enter 選択", "  인증 방법 선택: ↑/↓ 이동, Enter 선택", "  Выберите способ авторизации: ↑/↓ навигация, Enter выбрать")
	add(KeyProviderConnectInputHint, "  Base URL and API key: Tab switches fields, Enter confirms", "  Base URL 和 API key：按 Tab 切换字段，按 Enter 确认", "  Base URL und API-Schlüssel: Tab wechselt das Feld, Enter bestätigt", "  Base URL と API キー：Tab で項目切替、Enter で確定", "  Base URL 및 API 키: Tab으로 필드 전환, Enter로 확인", "  Base URL и API-ключ: Tab переключает поле, Enter подтверждает")
	add(KeyProviderConnectDefaultEndpoint, "_ (Provider default)", "_（Provider 默认值）", "_ (Provider-Standard)", "_（Provider のデフォルト）", "_ (Provider 기본값)", "_ (по умолчанию у Provider)")
	add(KeyProviderConnectBaseURL, "Base URL: %s", "Base URL：%s", "Base URL: %s", "Base URL：%s", "Base URL: %s", "Base URL: %s")
	add(KeyProviderConnectRequired, "_ (required)", "_（必填）", "_ (erforderlich)", "_（必須）", "_ (필수)", "_ (обязательно)")
	add(KeyProviderConnectAPIKey, "API key: %s", "API key：%s", "API-Schlüssel: %s", "API キー：%s", "API 키: %s", "API-ключ: %s")

	add(KeyModelTagVision, "vision", "视觉", "Bildverarbeitung", "画像", "비전", "изображения")
	add(KeyModelTagText, "text", "文本", "Text", "テキスト", "텍스트", "текст")
	add(KeyModelTagEffort, "effort", "推理强度", "Denkaufwand", "推論レベル", "추론 수준", "уровень рассуждений")
	add(KeyModelTagThinking, "thinking", "深度思考", "Denken", "思考", "사고", "рассуждения")
	add(KeyReasoningLabelLow, "Low", "低", "Niedrig", "低", "낮음", "Низкий")
	add(KeyReasoningLabelMedium, "Medium", "中", "Mittel", "中", "중간", "Средний")
	add(KeyReasoningLabelHigh, "High", "高", "Hoch", "高", "높음", "Высокий")
	add(KeyReasoningLabelExtraHigh, "Extra High", "超高", "Sehr hoch", "最高", "매우 높음", "Очень высокий")
	add(KeyReasoningLabelMax, "Max", "最大", "Maximal", "最大", "최대", "Максимум")
	add(KeyReasoningLabelDefault, "Default", "默认", "Standard", "デフォルト", "기본값", "По умолчанию")
	add(KeyReasoningDescriptionLow, "Fast responses with lighter reasoning", "更轻量的推理，响应更快", "Schnelle Antworten mit geringerem Denkaufwand", "軽い推論で高速に応答", "가벼운 추론으로 빠르게 응답", "Быстрые ответы с меньшей глубиной рассуждений")
	add(KeyReasoningDescriptionMedium, "Balances speed and reasoning depth for everyday tasks", "兼顾速度与推理深度，适合日常任务", "Ausgewogenes Verhältnis von Geschwindigkeit und Denktiefe für alltägliche Aufgaben", "日常タスク向けに速度と推論の深さを両立", "일상 작업에 적합한 속도와 추론 깊이의 균형", "Баланс скорости и глубины рассуждений для повседневных задач")
	add(KeyReasoningDescriptionHigh, "Greater reasoning depth for complex problems", "更深入地推理复杂问题", "Größere Denktiefe für komplexe Probleme", "複雑な問題をより深く推論", "복잡한 문제를 위한 더 깊은 추론", "Более глубокие рассуждения для сложных задач")
	add(KeyReasoningDescriptionExtraHigh, "Very high reasoning depth for complex problems", "以很高的推理深度处理复杂问题", "Sehr hohe Denktiefe für komplexe Probleme", "複雑な問題に非常に高い推論深度を使用", "복잡한 문제에 매우 높은 추론 깊이를 사용", "Очень высокая глубина рассуждений для сложных задач")
	add(KeyReasoningDescriptionMax, "Maximum reasoning depth for the hardest problems", "以最大推理深度处理最困难的问题", "Maximale Denktiefe für die schwierigsten Probleme", "最難関の問題に最大の推論深度を使用", "가장 어려운 문제에 최대 추론 깊이를 사용", "Максимальная глубина рассуждений для самых сложных задач")
	add(KeyReasoningDescriptionProviderDefined, "Provider-defined reasoning depth", "由 Provider 定义的推理深度", "Vom Provider definierte Denktiefe", "Provider が定義した推論の深さ", "Provider가 정의한 추론 깊이", "Глубина рассуждений задаётся Provider")

	add(KeyModelDescriptionGPT55, "Flagship model for complex reasoning and coding.", "适合复杂推理和编程的旗舰模型。", "Flaggschiffmodell für komplexe Schlussfolgerungen und Programmierung.", "複雑な推論とコーディングに適したフラッグシップモデル。", "복잡한 추론과 코딩을 위한 플래그십 모델입니다.", "Флагманская модель для сложных рассуждений и программирования.")
	add(KeyModelDescriptionGPT54, "Strong model for everyday coding.", "适合日常编程的强力模型。", "Leistungsstarkes Modell für alltägliche Programmieraufgaben.", "日常的なコーディングに強いモデル。", "일상적인 코딩에 강력한 모델입니다.", "Мощная модель для повседневного программирования.")
	add(KeyModelDescriptionGPT54Mini, "Small, fast, and cost-efficient model for simpler coding tasks.", "小巧、快速且经济，适合较简单的编程任务。", "Kleines, schnelles und kosteneffizientes Modell für einfachere Programmieraufgaben.", "小型・高速・低コストで、比較的簡単なコーディング向け。", "작고 빠르며 비용 효율적이고 간단한 코딩 작업에 적합합니다.", "Компактная, быстрая и экономичная модель для простых задач программирования.")
	add(KeyModelDescriptionGPT54Nano, "Smallest model for lightweight edits, triage, and low-latency tasks.", "最小巧的模型，适合轻量编辑、初步排查和低延迟任务。", "Kleinstes Modell für leichte Änderungen, Triage und Aufgaben mit niedriger Latenz.", "軽微な編集、トリアージ、低レイテンシのタスク向けの最小モデル。", "가벼운 편집, 분류, 저지연 작업을 위한 가장 작은 모델입니다.", "Самая компактная модель для небольших правок, первичного разбора и задач с низкой задержкой.")
	add(KeyModelDescriptionGPT53Codex, "Coding-optimized model for agentic coding tasks.", "针对 Agent 编程任务优化的模型。", "Für agentische Programmieraufgaben optimiertes Modell.", "Agent 型コーディング向けに最適化されたモデル。", "에이전트형 코딩 작업에 최적화된 모델입니다.", "Модель, оптимизированная для агентных задач программирования.")
	add(KeyModelDescriptionGPT52, "Previous frontier model for professional coding work.", "上一代前沿模型，适合专业编程工作。", "Vorheriges Spitzenmodell für professionelle Programmierarbeit.", "プロ向けコーディング作業に適した前世代の最上位モデル。", "전문 코딩 작업을 위한 이전 세대 프런티어 모델입니다.", "Предыдущая передовая модель для профессионального программирования.")
	add(KeyModelDescriptionGPT5, "Strong general-purpose model for coding and agentic tasks.", "适合编程和 Agent 任务的强力通用模型。", "Leistungsstarkes Allzweckmodell für Programmierung und agentische Aufgaben.", "コーディングと Agent タスクに強い汎用モデル。", "코딩과 에이전트 작업에 강력한 범용 모델입니다.", "Мощная универсальная модель для программирования и агентных задач.")
	add(KeyModelDescriptionGPT5Mini, "Fast, cost-efficient model for simpler coding tasks.", "快速且经济，适合较简单的编程任务。", "Schnelles, kosteneffizientes Modell für einfachere Programmieraufgaben.", "高速かつ低コストで、比較的簡単なコーディング向け。", "빠르고 비용 효율적이며 간단한 코딩 작업에 적합합니다.", "Быстрая и экономичная модель для простых задач программирования.")
	add(KeyModelDescriptionGPT4O, "Fast multimodal model for general chat and vision tasks.", "快速多模态模型，适合通用对话和视觉任务。", "Schnelles multimodales Modell für allgemeine Chats und Bildaufgaben.", "一般的な対話と画像タスク向けの高速マルチモーダルモデル。", "일반 대화와 비전 작업을 위한 빠른 멀티모달 모델입니다.", "Быстрая мультимодальная модель для общего диалога и задач с изображениями.")
	add(KeyModelDescriptionGPT4OMini, "Low-cost multimodal model for lightweight tasks.", "低成本多模态模型，适合轻量任务。", "Kostengünstiges multimodales Modell für leichte Aufgaben.", "軽量タスク向けの低コストなマルチモーダルモデル。", "가벼운 작업을 위한 저비용 멀티모달 모델입니다.", "Недорогая мультимодальная модель для лёгких задач.")
	add(KeyModelDescriptionCodexSpark, "Ultra-fast coding model.", "超高速编程模型。", "Extrem schnelles Programmiermodell.", "超高速なコーディングモデル。", "초고속 코딩 모델입니다.", "Сверхбыстрая модель для программирования.")
	add(KeyModelDescriptionCodex, "Coding-optimized model.", "针对编程优化的模型。", "Für Programmierung optimiertes Modell.", "コーディング向けに最適化されたモデル。", "코딩에 최적화된 모델입니다.", "Модель, оптимизированная для программирования.")
	add(KeyModelDescriptionDeepSeekPro, "DeepSeek model for complex coding and reasoning tasks.", "适合复杂编程和推理任务的 DeepSeek 模型。", "DeepSeek-Modell für komplexe Programmier- und Denkaufgaben.", "複雑なコーディングと推論タスク向けの DeepSeek モデル。", "복잡한 코딩과 추론 작업을 위한 DeepSeek 모델입니다.", "Модель DeepSeek для сложного программирования и рассуждений.")
	add(KeyModelDescriptionDeepSeekFlash, "Fast DeepSeek model for everyday coding and agent tasks.", "适合日常编程和 Agent 任务的快速 DeepSeek 模型。", "Schnelles DeepSeek-Modell für alltägliche Programmier- und Agentenaufgaben.", "日常的なコーディングと Agent タスク向けの高速 DeepSeek モデル。", "일상 코딩과 에이전트 작업을 위한 빠른 DeepSeek 모델입니다.", "Быстрая модель DeepSeek для повседневного программирования и агентных задач.")
	add(KeyModelDescriptionClaudeSonnet, "Balanced Claude model for everyday coding and agent workflows.", "均衡的 Claude 模型，适合日常编程和 Agent 工作流。", "Ausgewogenes Claude-Modell für alltägliche Programmier- und Agentenabläufe.", "日常的なコーディングと Agent ワークフロー向けのバランス型 Claude モデル。", "일상 코딩과 에이전트 워크플로를 위한 균형 잡힌 Claude 모델입니다.", "Сбалансированная модель Claude для повседневного программирования и агентных процессов.")
	add(KeyModelDescriptionClaudeOpus, "Claude's strongest model for complex coding and long-horizon work.", "Claude 最强模型，适合复杂编程和长期任务。", "Stärkstes Claude-Modell für komplexe Programmierung und langfristige Aufgaben.", "複雑なコーディングと長期的な作業に適した Claude の最上位モデル。", "복잡한 코딩과 장기 작업을 위한 Claude의 가장 강력한 모델입니다.", "Самая мощная модель Claude для сложного программирования и длительных задач.")
	add(KeyModelDescriptionClaudeHaiku, "Fast Claude model for lightweight, cost-sensitive tasks.", "快速 Claude 模型，适合轻量且成本敏感的任务。", "Schnelles Claude-Modell für leichte, kostensensible Aufgaben.", "軽量でコスト重視のタスク向けの高速 Claude モデル。", "가볍고 비용에 민감한 작업을 위한 빠른 Claude 모델입니다.", "Быстрая модель Claude для лёгких задач с ограниченным бюджетом.")
	add(KeyModelDescriptionGeminiPro, "Gemini thinking model for complex code, math, and long context.", "适合复杂代码、数学和长上下文的 Gemini 思考模型。", "Gemini-Denkmodell für komplexen Code, Mathematik und langen Kontext.", "複雑なコード、数学、長いコンテキスト向けの Gemini 思考モデル。", "복잡한 코드, 수학, 긴 컨텍스트를 위한 Gemini 사고 모델입니다.", "Модель Gemini для сложного кода, математики и длинного контекста.")
	add(KeyModelDescriptionGeminiFlashLite, "Low-latency Gemini multimodal model for high-volume simple tasks.", "低延迟 Gemini 多模态模型，适合大量简单任务。", "Gemini-Multimodalmodell mit niedriger Latenz für viele einfache Aufgaben.", "大量の単純タスク向けの低レイテンシ Gemini マルチモーダルモデル。", "대량의 간단한 작업을 위한 저지연 Gemini 멀티모달 모델입니다.", "Мультимодальная модель Gemini с низкой задержкой для большого объёма простых задач.")
	add(KeyModelDescriptionGeminiFlash, "Fast Gemini multimodal model balancing cost and speed.", "兼顾成本与速度的快速 Gemini 多模态模型。", "Schnelles Gemini-Multimodalmodell mit ausgewogenem Verhältnis von Kosten und Geschwindigkeit.", "コストと速度のバランスに優れた高速 Gemini マルチモーダルモデル。", "비용과 속도의 균형을 맞춘 빠른 Gemini 멀티모달 모델입니다.", "Быстрая мультимодальная модель Gemini с балансом стоимости и скорости.")
	add(KeyModelDescriptionReasoning, "Reasoning-capable model for more complex tasks.", "具备推理能力，适合更复杂的任务。", "Denkfähiges Modell für komplexere Aufgaben.", "より複雑なタスクに対応できる推論モデル。", "더 복잡한 작업을 위한 추론 가능 모델입니다.", "Модель с поддержкой рассуждений для более сложных задач.")
	add(KeyModelDescriptionMultimodal, "Multimodal model for text and image inputs.", "支持文本和图像输入的多模态模型。", "Multimodales Modell für Text- und Bildeingaben.", "テキストと画像入力に対応するマルチモーダルモデル。", "텍스트와 이미지 입력을 위한 멀티모달 모델입니다.", "Мультимодальная модель для текстовых и графических данных.")
	add(KeyModelDescriptionGeneral, "General-purpose model for coding tasks.", "适合编程任务的通用模型。", "Allzweckmodell für Programmieraufgaben.", "コーディングタスク向けの汎用モデル。", "코딩 작업을 위한 범용 모델입니다.", "Универсальная модель для задач программирования.")
}
