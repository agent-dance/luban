package i18n

const (
	KeyToolWebSummariserUnavailable  Key = "tool.web.summariser.unavailable"
	KeyToolWebSummariserFailed       Key = "tool.web.summariser.failed"
	KeyToolWebNoModelResponse        Key = "tool.web.summariser.no_response"
	KeyToolWebSearchError            Key = "tool.web.search.error"
	KeyToolWebServerToolFailed       Key = "tool.web.server_tool.failed"
	KeyToolMCPFormatJSONSchema       Key = "tool.mcp.format.json_schema"
	KeyToolMCPFormatJSON             Key = "tool.mcp.format.json"
	KeyToolMCPFormatJSONArraySchema  Key = "tool.mcp.format.json_array_schema"
	KeyToolMCPFormatJSONArray        Key = "tool.mcp.format.json_array"
	KeyToolMCPFormatPlainText        Key = "tool.mcp.format.plain_text"
	KeyToolMCPLargeOutputStored      Key = "tool.mcp.large_output.stored"
	KeyToolVerificationNudge         Key = "tool.verification.nudge"
	KeyToolWebRedirectMarker         Key = "tool.web.redirect.marker"
	KeyToolWebSearchResultsHeader    Key = "tool.web.search.results_header"
	KeyToolWebSearchLinks            Key = "tool.web.search.links"
	KeyToolWebSearchSourcesReminder  Key = "tool.web.search.sources_reminder"
	KeyToolWebSearchSourcesHeading   Key = "tool.web.search.sources_heading"
	KeyToolMCPPaginationHint         Key = "tool.mcp.pagination_hint"
	KeyToolMCPTruncationHint         Key = "tool.mcp.truncation_hint"
	KeyToolMCPReadServerURIRequired  Key = "tool.mcp.read.server_uri_required"
	KeyToolMCPReadServerRequired     Key = "tool.mcp.read.server_required"
	KeyToolMCPReadURIRequired        Key = "tool.mcp.read.uri_required"
	KeyToolMCPReadInvalidInput       Key = "tool.mcp.read.invalid_input"
	KeyToolMCPReadNotConnected       Key = "tool.mcp.read.not_connected"
	KeyToolMCPReadNotConnectedCause  Key = "tool.mcp.read.not_connected_cause"
	KeyToolMCPReadUnsupported        Key = "tool.mcp.read.unsupported"
	KeyToolMCPReadFailed             Key = "tool.mcp.read.failed"
	KeyToolMCPReadInvalidResult      Key = "tool.mcp.read.invalid_result"
	KeyToolMCPReadMarshalResult      Key = "tool.mcp.read.marshal_result"
	KeyToolMCPReadEncodeRequest      Key = "tool.mcp.read.encode_request"
	KeyToolMCPReadGenericError       Key = "tool.mcp.read.generic_error"
	KeyToolMCPReadHTTPResponse       Key = "tool.mcp.read.http_response"
	KeyToolMCPReadOAuthRequired      Key = "tool.mcp.read.oauth_required"
	KeyToolMCPReadInvalidJSONRPC     Key = "tool.mcp.read.invalid_json_rpc"
	KeyToolMCPReadRPCFailed          Key = "tool.mcp.read.rpc_failed"
	KeyToolMCPReadMissingResult      Key = "tool.mcp.read.missing_result"
	KeyToolMCPDynamicUninitialized   Key = "tool.mcp.dynamic.uninitialized"
	KeyToolMCPReadContentURIRequired Key = "tool.mcp.read.content_uri_required"
	KeyToolMCPReadInvalidBase64      Key = "tool.mcp.read.invalid_base64"
	KeyToolMCPReadBinarySaveFailed   Key = "tool.mcp.read.binary_save_failed"
	KeyToolMCPUnsafeOutputPath       Key = "tool.mcp.output.unsafe_path"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyToolWebSummariserUnavailable, "WebFetch summariser is not configured", "尚未配置 WebFetch summariser", "WebFetch-Summariser ist nicht konfiguriert", "WebFetch summariser が設定されていません", "WebFetch summariser가 구성되지 않았습니다", "WebFetch summariser не настроен")
	add(KeyToolWebSummariserFailed, "WebFetch summariser failed: %v", "WebFetch summariser 失败：%v", "WebFetch-Summariser fehlgeschlagen: %v", "WebFetch summariser に失敗しました: %v", "WebFetch summariser 실패: %v", "Ошибка WebFetch summariser: %v")
	add(KeyToolWebNoModelResponse, "No response from model", "Model 未返回响应", "Keine Antwort vom Modell", "Model から応答がありません", "Model의 응답이 없습니다", "Model не вернула ответ")
	add(KeyToolWebSearchError, "Web search error: %s", "Web search 错误：%s", "Web-Suchfehler: %s", "Web search エラー: %s", "Web search 오류: %s", "Ошибка Web search: %s")
	add(KeyToolWebServerToolFailed, "web_fetch server tool failed: %v", "web_fetch server tool 失败：%v", "web_fetch-Server-Tool fehlgeschlagen: %v", "web_fetch server tool に失敗しました: %v", "web_fetch server tool 실패: %v", "Ошибка web_fetch server tool: %v")
	add(KeyToolMCPFormatJSONSchema, "JSON with schema: %s", "带 schema 的 JSON：%s", "JSON mit Schema: %s", "schema 付き JSON: %s", "schema가 있는 JSON: %s", "JSON со schema: %s")
	add(KeyToolMCPFormatJSON, "JSON", "JSON", "JSON", "JSON", "JSON", "JSON")
	add(KeyToolMCPFormatJSONArraySchema, "JSON array with schema: %s", "带 schema 的 JSON 数组：%s", "JSON-Array mit Schema: %s", "schema 付き JSON 配列: %s", "schema가 있는 JSON 배열: %s", "Массив JSON со schema: %s")
	add(KeyToolMCPFormatJSONArray, "JSON array", "JSON 数组", "JSON-Array", "JSON 配列", "JSON 배열", "Массив JSON")
	add(KeyToolMCPFormatPlainText, "Plain text", "纯文本", "Klartext", "プレーンテキスト", "일반 텍스트", "Обычный текст")
	add(KeyToolMCPLargeOutputStored,
		"Error: result (%s characters) exceeds maximum allowed tokens. Output has been saved to %s.\nFormat: %s\nUse offset and limit parameters to read specific portions of the file, search within it for specific content, and jq to make structured queries.\nREQUIREMENTS FOR SUMMARIZATION/ANALYSIS/REVIEW:\n- You MUST read the content from the file at %s in sequential chunks until 100%% of the content has been read.\n- If you receive truncation warnings when reading the file, reduce the chunk size until you have read 100%% of the content without truncation.\n- Before producing ANY summary or analysis, you MUST explicitly describe what portion of the content you have read. ***If you did not read the entire content, you MUST explicitly state this.***\n",
		"错误：结果（%s 个字符）超出允许的 token 上限。输出已保存至 %s。\n格式：%s\n请使用 offset 和 limit 参数读取文件的特定部分、搜索特定内容，并使用 jq 进行结构化查询。\n摘要/分析/审查要求：\n- 必须按顺序分块读取 %s 中的内容，直至读取 100%%。\n- 如果读取时收到截断警告，请缩小分块大小，直到无截断地读取全部内容。\n- 在生成任何摘要或分析前，必须明确说明已读取的范围。***如果未读取全部内容，必须明确指出。***\n",
		"Fehler: Das Ergebnis (%s Zeichen) überschreitet die maximal zulässige Token-Zahl. Die Ausgabe wurde unter %s gespeichert.\nFormat: %s\nVerwende offset und limit, um bestimmte Dateibereiche zu lesen, suche gezielt nach Inhalten und nutze jq für strukturierte Abfragen.\nANFORDERUNGEN FÜR ZUSAMMENFASSUNG/ANALYSE/PRÜFUNG:\n- Du MUSST den Inhalt der Datei %s in aufeinanderfolgenden Blöcken lesen, bis 100%% erfasst wurden.\n- Bei Kürzungswarnungen verkleinere die Blöcke, bis der gesamte Inhalt ohne Kürzung gelesen wurde.\n- Vor JEDER Zusammenfassung oder Analyse MUSST du ausdrücklich angeben, welchen Teil du gelesen hast. ***Wenn du nicht den gesamten Inhalt gelesen hast, MUSST du dies ausdrücklich sagen.***\n",
		"エラー: 結果（%s 文字）が許容 token 上限を超えています。出力を %s に保存しました。\n形式: %s\noffset と limit で必要な範囲を読み、特定の内容を検索し、構造化クエリには jq を使用してください。\n要約/分析/レビューの要件:\n- %s の内容を順番に分割して、100%% 読み終えるまで必ず読み込んでください。\n- 読み込み時に切り詰め警告が出た場合は、切り詰めなしで全内容を読めるまで分割サイズを小さくしてください。\n- 要約や分析を作成する前に、読んだ範囲を必ず明示してください。***全体を読んでいない場合は、その旨を必ず明記してください。***\n",
		"오류: 결과(%s자)가 허용된 최대 token 수를 초과했습니다. 출력이 %s에 저장되었습니다.\n형식: %s\noffset과 limit로 파일의 특정 부분을 읽고, 필요한 내용을 검색하며, 구조화된 쿼리에는 jq를 사용하세요.\n요약/분석/검토 요구 사항:\n- %s의 내용을 순서대로 나누어 100%%를 읽을 때까지 반드시 읽어야 합니다.\n- 읽는 중 잘림 경고가 나오면 전체 내용을 잘림 없이 읽을 때까지 청크 크기를 줄이세요.\n- 요약이나 분석을 작성하기 전에 읽은 범위를 반드시 명시하세요. ***전체 내용을 읽지 않았다면 반드시 명확히 밝혀야 합니다.***\n",
		"Ошибка: результат (%s символов) превышает допустимый предел token. Вывод сохранён в %s.\nФормат: %s\nИспользуйте offset и limit для чтения нужных частей файла, поиск для конкретного содержимого и jq для структурированных запросов.\nТРЕБОВАНИЯ К РЕЗЮМЕ/АНАЛИЗУ/ПРОВЕРКЕ:\n- Вы ОБЯЗАНЫ последовательно читать содержимое файла %s блоками, пока не будет прочитано 100%%.\n- При предупреждениях об усечении уменьшайте размер блока, пока всё содержимое не будет прочитано без усечения.\n- Перед ЛЮБЫМ резюме или анализом явно укажите, какую часть содержимого вы прочитали. ***Если прочитано не всё, это ОБЯЗАТЕЛЬНО нужно явно указать.***\n")
	add(KeyToolVerificationNudge,
		`\n\nNOTE: You just closed out 3+ tasks and none of them was a verification step. Before writing your final summary, spawn the verification agent (subagent_type="%s"). You cannot self-assign PARTIAL by listing caveats in your summary — only the verifier issues a verdict.`,
		`\n\n注意：你刚刚完成了至少 3 个任务，但其中没有验证步骤。在撰写最终总结前，请生成 verification Agent（subagent_type="%s"）。不能仅在总结中列出限制就自行判定为 PARTIAL；只有 verifier 可以给出结论。`,
		`\n\nHINWEIS: Du hast gerade mindestens 3 Aufgaben abgeschlossen, ohne einen Verifizierungsschritt auszuführen. Starte vor der abschließenden Zusammenfassung den Verifizierungs-Agent (subagent_type="%s"). Du kannst dir PARTIAL nicht selbst durch Hinweise in der Zusammenfassung zuweisen; nur der Verifier gibt ein Urteil ab.`,
		`\n\n注意: 3 件以上のタスクを完了しましたが、検証ステップがありません。最終まとめを書く前に verification Agent（subagent_type="%s"）を起動してください。まとめに注意事項を列挙して PARTIAL を自己判定することはできません。判定できるのは verifier だけです。`,
		`\n\n참고: 방금 3개 이상의 작업을 완료했지만 검증 단계가 없었습니다. 최종 요약을 작성하기 전에 verification Agent(subagent_type="%s")를 시작하세요. 요약에 주의 사항을 나열하는 것만으로 PARTIAL을 자체 판정할 수 없으며, verifier만 판정할 수 있습니다.`,
		`\n\nПРИМЕЧАНИЕ: Вы завершили не менее 3 задач, но ни одна из них не была проверкой. Перед итоговым резюме запустите verification Agent (subagent_type="%s"). Нельзя самостоятельно назначить PARTIAL, просто перечислив оговорки; вердикт выносит только verifier.`)
	add(KeyToolWebRedirectMarker,
		"REDIRECT DETECTED: The URL redirects to a different host.\n\nOriginal URL: %s\nRedirect URL: %s\nStatus: %d %s\n\nTo complete your request, I need to fetch content from the redirected URL. Please use WebFetch again with these parameters:\n- url: %q\n- prompt: %q",
		"检测到重定向：URL 跳转到了其他 host。\n\n原始 URL：%s\n重定向 URL：%s\n状态：%d %s\n\n要完成请求，需要从重定向后的 URL 获取内容。请使用以下参数再次调用 WebFetch：\n- url: %q\n- prompt: %q",
		"WEITERLEITUNG ERKANNT: Die URL leitet zu einem anderen Host weiter.\n\nUrsprüngliche URL: %s\nWeiterleitungs-URL: %s\nStatus: %d %s\n\nZum Abschließen der Anfrage muss der Inhalt von der weitergeleiteten URL abgerufen werden. Rufe WebFetch mit diesen Parametern erneut auf:\n- url: %q\n- prompt: %q",
		"リダイレクトを検出: URL は別の host にリダイレクトされます。\n\n元の URL: %s\nリダイレクト先 URL: %s\nステータス: %d %s\n\nリクエストを完了するには、リダイレクト先 URL から内容を取得する必要があります。次のパラメーターで WebFetch を再度使用してください:\n- url: %q\n- prompt: %q",
		"리디렉션 감지: URL이 다른 host로 리디렉션됩니다.\n\n원래 URL: %s\n리디렉션 URL: %s\n상태: %d %s\n\n요청을 완료하려면 리디렉션된 URL에서 콘텐츠를 가져와야 합니다. 다음 매개변수로 WebFetch를 다시 사용하세요:\n- url: %q\n- prompt: %q",
		"ОБНАРУЖЕНО ПЕРЕНАПРАВЛЕНИЕ: URL ведёт на другой host.\n\nИсходный URL: %s\nURL перенаправления: %s\nСтатус: %d %s\n\nЧтобы завершить запрос, нужно получить содержимое по новому URL. Снова вызовите WebFetch с параметрами:\n- url: %q\n- prompt: %q")
	add(KeyToolWebSearchResultsHeader, "Web search results for query: %q\n\n", "Web search 查询 %q 的结果：\n\n", "Web-Suchergebnisse für die Anfrage %q:\n\n", "Web search クエリ %q の結果:\n\n", "Web search 쿼리 %q 결과:\n\n", "Результаты Web search для запроса %q:\n\n")
	add(KeyToolWebSearchLinks, "Links: %s\n\n", "链接：%s\n\n", "Links: %s\n\n", "リンク: %s\n\n", "링크: %s\n\n", "Ссылки: %s\n\n")
	add(KeyToolWebSearchSourcesReminder, "REMINDER: You MUST include the sources above in your response to the user using markdown hyperlinks.", "提醒：回复用户时必须使用 Markdown 超链接引用上述来源。", "ERINNERUNG: Du MUSST die obigen Quellen in deiner Antwort mit Markdown-Hyperlinks angeben.", "注意: ユーザーへの回答には、上記の出典を Markdown のハイパーリンクで必ず含めてください。", "알림: 사용자에게 응답할 때 위 출처를 Markdown 하이퍼링크로 반드시 포함하세요.", "НАПОМИНАНИЕ: В ответе пользователю ОБЯЗАТЕЛЬНО укажите приведённые выше источники в виде Markdown-ссылок.")
	add(KeyToolWebSearchSourcesHeading, "Sources:\n", "来源：\n", "Quellen:\n", "出典:\n", "출처:\n", "Источники:\n")
	add(KeyToolMCPPaginationHint, " If this MCP server provides pagination or filtering tools, use them to retrieve specific portions of the data.", " 如果此 MCP server 提供分页或筛选工具，请用它们获取所需的数据片段。", " Wenn dieser MCP-Server Tools zum Paginieren oder Filtern anbietet, verwende sie, um bestimmte Teile der Daten abzurufen.", " この MCP server がページ分割または絞り込み用ツールを提供している場合は、それを使って必要なデータ範囲を取得してください。", " 이 MCP server가 페이지 나누기 또는 필터링 도구를 제공한다면 이를 사용해 필요한 데이터 부분을 가져오세요.", " Если MCP server предоставляет инструменты пагинации или фильтрации, используйте их для получения нужных частей данных.")
	add(KeyToolMCPTruncationHint, " If this MCP server provides pagination or filtering tools, use them to retrieve specific portions of the data. If pagination is not available, inform the user that you are working with truncated output and results may be incomplete.", " 如果此 MCP server 提供分页或筛选工具，请用它们获取所需的数据片段。如果无法分页，请告知用户当前使用的是截断后的输出，结果可能不完整。", " Wenn dieser MCP-Server Tools zum Paginieren oder Filtern anbietet, verwende sie, um bestimmte Teile der Daten abzurufen. Ist keine Paginierung verfügbar, weise den Benutzer darauf hin, dass die Ausgabe gekürzt ist und Ergebnisse unvollständig sein können.", " この MCP server がページ分割または絞り込み用ツールを提供している場合は、それを使って必要なデータ範囲を取得してください。ページ分割できない場合は、出力が切り詰められており結果が不完全な可能性があることをユーザーに伝えてください。", " 이 MCP server가 페이지 나누기 또는 필터링 도구를 제공한다면 이를 사용해 필요한 데이터 부분을 가져오세요. 페이지 나누기를 사용할 수 없다면 잘린 출력을 사용 중이며 결과가 불완전할 수 있음을 사용자에게 알리세요.", " Если MCP server предоставляет инструменты пагинации или фильтрации, используйте их для получения нужных частей данных. Если пагинация недоступна, сообщите пользователю, что вывод усечён и результаты могут быть неполными.")
	add(KeyToolMCPReadServerURIRequired, "server and uri are required", "必须提供 server 和 uri", "server und uri sind erforderlich", "server と uri は必須です", "server와 uri가 필요합니다", "Требуются server и uri")
	add(KeyToolMCPReadServerRequired, "server is required", "必须提供 server", "server ist erforderlich", "server は必須です", "server가 필요합니다", "Требуется server")
	add(KeyToolMCPReadURIRequired, "uri is required", "必须提供 uri", "uri ist erforderlich", "uri は必須です", "uri가 필요합니다", "Требуется uri")
	add(KeyToolMCPReadInvalidInput, "Error: invalid input: %s", "错误：输入无效：%s", "Fehler: ungültige Eingabe: %s", "エラー: 入力が無効です: %s", "오류: 잘못된 입력: %s", "Ошибка: недопустимые входные данные: %s")
	add(KeyToolMCPReadNotConnected, "Server %q is not connected", "Server %q 未连接", "Server %q ist nicht verbunden", "Server %q は接続されていません", "Server %q이(가) 연결되지 않았습니다", "Server %q не подключён")
	add(KeyToolMCPReadNotConnectedCause, "Server %q is not connected: %v", "Server %q 未连接：%v", "Server %q ist nicht verbunden: %v", "Server %q は接続されていません: %v", "Server %q이(가) 연결되지 않았습니다: %v", "Server %q не подключён: %v")
	add(KeyToolMCPReadUnsupported, "Server %q does not support resources", "Server %q 不支持 resources", "Server %q unterstützt keine Ressourcen", "Server %q は resources をサポートしていません", "Server %q은(는) resources를 지원하지 않습니다", "Server %q не поддерживает resources")
	add(KeyToolMCPReadFailed, "Error: reading resource %q from %q: %s", "错误：从 %[2]q 读取 resource %[1]q 失败：%[3]s", "Fehler beim Lesen der Ressource %q von %q: %s", "%[2]q から resource %[1]q を読み取れませんでした: %[3]s", "%[2]q에서 resource %[1]q을(를) 읽는 중 오류: %[3]s", "Ошибка чтения resource %q с %q: %s")
	add(KeyToolMCPReadInvalidResult, "Error: invalid resources/read result: %s", "错误：resources/read 结果无效：%s", "Fehler: ungültiges resources/read-Ergebnis: %s", "エラー: resources/read の結果が無効です: %s", "오류: 잘못된 resources/read 결과: %s", "Ошибка: недопустимый результат resources/read: %s")
	add(KeyToolMCPReadMarshalResult, "Error: marshal resources/read result: %s", "错误：无法序列化 resources/read 结果：%s", "Fehler beim Serialisieren des resources/read-Ergebnisses: %s", "エラー: resources/read の結果をシリアル化できませんでした: %s", "오류: resources/read 결과를 직렬화하지 못했습니다: %s", "Ошибка сериализации результата resources/read: %s")
	add(KeyToolMCPReadEncodeRequest, "Error: encode MCP resources/read request: %s", "错误：无法编码 MCP resources/read 请求：%s", "Fehler beim Codieren der MCP-resources/read-Anfrage: %s", "エラー: MCP resources/read リクエストをエンコードできませんでした: %s", "오류: MCP resources/read 요청을 인코딩하지 못했습니다: %s", "Ошибка кодирования запроса MCP resources/read: %s")
	add(KeyToolMCPReadGenericError, "Error: %s", "错误：%s", "Fehler: %s", "エラー: %s", "오류: %s", "Ошибка: %s")
	add(KeyToolMCPReadHTTPResponse, "Error: read MCP HTTP response: %s", "错误：无法读取 MCP HTTP 响应：%s", "Fehler beim Lesen der MCP-HTTP-Antwort: %s", "エラー: MCP HTTP レスポンスを読み取れませんでした: %s", "오류: MCP HTTP 응답을 읽지 못했습니다: %s", "Ошибка чтения ответа MCP HTTP: %s")
	add(KeyToolMCPReadOAuthRequired, "MCP server requires OAuth authorization; complete handshake before retrying", "MCP server 需要 OAuth 授权；请完成握手后再重试", "Der MCP-Server erfordert eine OAuth-Autorisierung; schließe den Handshake vor dem nächsten Versuch ab", "MCP server には OAuth 認証が必要です。ハンドシェイクを完了してから再試行してください", "MCP server에 OAuth 인증이 필요합니다. 핸드셰이크를 완료한 뒤 다시 시도하세요", "MCP server требует авторизации OAuth; завершите handshake перед повторной попыткой")
	add(KeyToolMCPReadInvalidJSONRPC, "Error: invalid MCP JSON-RPC response: %s", "错误：MCP JSON-RPC 响应无效：%s", "Fehler: ungültige MCP-JSON-RPC-Antwort: %s", "エラー: MCP JSON-RPC レスポンスが無効です: %s", "오류: 잘못된 MCP JSON-RPC 응답: %s", "Ошибка: недопустимый ответ MCP JSON-RPC: %s")
	add(KeyToolMCPReadRPCFailed, "Error: MCP resources/read failed (%d): %s", "错误：MCP resources/read 失败（%d）：%s", "Fehler: MCP resources/read fehlgeschlagen (%d): %s", "エラー: MCP resources/read に失敗しました（%d）: %s", "오류: MCP resources/read 실패(%d): %s", "Ошибка MCP resources/read (%d): %s")
	add(KeyToolMCPReadMissingResult, "Error: MCP resources/read response missing result", "错误：MCP resources/read 响应缺少 result", "Fehler: In der MCP-resources/read-Antwort fehlt result", "エラー: MCP resources/read レスポンスに result がありません", "오류: MCP resources/read 응답에 result가 없습니다", "Ошибка: в ответе MCP resources/read отсутствует result")
	add(KeyToolMCPDynamicUninitialized, "Error: MCP dynamic tool is not initialized", "错误：MCP dynamic tool 尚未初始化", "Fehler: Das dynamische MCP-Tool ist nicht initialisiert", "エラー: MCP dynamic tool が初期化されていません", "오류: MCP dynamic tool이 초기화되지 않았습니다", "Ошибка: динамический MCP tool не инициализирован")
	add(KeyToolMCPReadContentURIRequired, "contents[%d]: uri is required", "contents[%d]：必须提供 uri", "contents[%d]: uri ist erforderlich", "contents[%d]: uri は必須です", "contents[%d]: uri가 필요합니다", "contents[%d]: требуется uri")
	add(KeyToolMCPReadInvalidBase64, "Binary content could not be saved to disk: invalid base64 content: %s", "无法将二进制内容保存到磁盘：base64 内容无效：%s", "Binärinhalt konnte nicht gespeichert werden: ungültiger Base64-Inhalt: %s", "バイナリデータをディスクに保存できませんでした: base64 データが無効です: %s", "바이너리 콘텐츠를 디스크에 저장하지 못했습니다: 잘못된 base64 콘텐츠: %s", "Не удалось сохранить двоичные данные на диск: недопустимое содержимое base64: %s")
	add(KeyToolMCPReadBinarySaveFailed, "Binary content could not be saved to disk: %s", "无法将二进制内容保存到磁盘：%s", "Binärinhalt konnte nicht gespeichert werden: %s", "バイナリデータをディスクに保存できませんでした: %s", "바이너리 콘텐츠를 디스크에 저장하지 못했습니다: %s", "Не удалось сохранить двоичные данные на диск: %s")
	add(KeyToolMCPUnsafeOutputPath, "unsafe MCP output path", "MCP 输出路径不安全", "Unsicherer MCP-Ausgabepfad", "安全でない MCP 出力パス", "안전하지 않은 MCP 출력 경로", "Небезопасный путь вывода MCP")
}
