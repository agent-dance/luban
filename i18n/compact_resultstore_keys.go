package i18n

// Semantic errors raised while oversized tool results are persisted. File
// paths, tool-use IDs, and operating-system errors remain raw format values so
// diagnostics are not altered by translation.
const (
	KeyCompactResultStoreUnavailable         Key = "compact.resultstore.unavailable"
	KeyCompactResultStoreCreateRawDirectory  Key = "compact.resultstore.create_raw_directory"
	KeyCompactResultStoreCreateRawFile       Key = "compact.resultstore.create_raw_file"
	KeyCompactResultStoreWriteRawFile        Key = "compact.resultstore.write_raw_file"
	KeyCompactResultStoreCloseRawFile        Key = "compact.resultstore.close_raw_file"
	KeyCompactResultStoreCreateResultFile    Key = "compact.resultstore.create_result_file"
	KeyCompactResultStoreWriteResultFile     Key = "compact.resultstore.write_result_file"
	KeyCompactResultStoreCloseResultFile     Key = "compact.resultstore.close_result_file"
	KeyCompactResultStoreSerializeStructured Key = "compact.resultstore.serialize_structured"
)

var compactResultStoreKeys = []Key{
	KeyCompactResultStoreUnavailable,
	KeyCompactResultStoreCreateRawDirectory,
	KeyCompactResultStoreCreateRawFile,
	KeyCompactResultStoreWriteRawFile,
	KeyCompactResultStoreCloseRawFile,
	KeyCompactResultStoreCreateResultFile,
	KeyCompactResultStoreWriteResultFile,
	KeyCompactResultStoreCloseResultFile,
	KeyCompactResultStoreSerializeStructured,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyCompactResultStoreUnavailable,
		"persist raw tool output: result store is nil",
		"无法持久化工具原始输出：结果存储未初始化",
		"Die Rohausgabe des Tools konnte nicht gespeichert werden: Der Ergebnisspeicher ist nicht initialisiert",
		"ツールの生出力を保存できません：結果ストアが初期化されていません",
		"도구 원본 출력을 저장할 수 없습니다: 결과 저장소가 초기화되지 않았습니다",
		"Не удалось сохранить необработанный вывод инструмента: хранилище результатов не инициализировано")
	add(KeyCompactResultStoreCreateRawDirectory,
		"persist raw tool output: %v",
		"无法创建工具原始输出的存储目录：%v",
		"Das Verzeichnis für die Rohausgabe des Tools konnte nicht erstellt werden: %v",
		"ツールの生出力を保存するディレクトリを作成できませんでした：%v",
		"도구 원본 출력 저장 디렉터리를 만들 수 없습니다: %v",
		"Не удалось создать каталог для необработанного вывода инструмента: %v")
	add(KeyCompactResultStoreCreateRawFile,
		"persist raw tool output: %v",
		"无法创建工具原始输出文件：%v",
		"Die Datei für die Rohausgabe des Tools konnte nicht erstellt werden: %v",
		"ツールの生出力ファイルを作成できませんでした：%v",
		"도구 원본 출력 파일을 만들 수 없습니다: %v",
		"Не удалось создать файл для необработанного вывода инструмента: %v")
	add(KeyCompactResultStoreWriteRawFile,
		"persist raw tool output to %s: %v",
		"无法将工具原始输出写入 %s：%v",
		"Die Rohausgabe des Tools konnte nicht in %s geschrieben werden: %v",
		"ツールの生出力を %s に書き込めませんでした：%v",
		"도구 원본 출력을 %s에 쓸 수 없습니다: %v",
		"Не удалось записать необработанный вывод инструмента в %s: %v")
	add(KeyCompactResultStoreCloseRawFile,
		"persist raw tool output to %s: %v",
		"无法完成工具原始输出文件 %s 的写入：%v",
		"Das Schreiben der Rohausgabe des Tools nach %s konnte nicht abgeschlossen werden: %v",
		"ツールの生出力ファイル %s への書き込みを完了できませんでした：%v",
		"도구 원본 출력 파일 %s 쓰기를 완료할 수 없습니다: %v",
		"Не удалось завершить запись необработанного вывода инструмента в %s: %v")
	add(KeyCompactResultStoreCreateResultFile,
		"persist tool result to %s: %v",
		"无法在 %s 创建工具结果文件：%v",
		"Die Datei für das Tool-Ergebnis konnte unter %s nicht erstellt werden: %v",
		"ツール結果ファイルを %s に作成できませんでした：%v",
		"도구 결과 파일을 %s에 만들 수 없습니다: %v",
		"Не удалось создать файл результата инструмента в %s: %v")
	add(KeyCompactResultStoreWriteResultFile,
		"persist tool result to %s: %v",
		"无法将工具结果写入 %s：%v",
		"Das Tool-Ergebnis konnte nicht nach %s geschrieben werden: %v",
		"ツール結果を %s に書き込めませんでした：%v",
		"도구 결과를 %s에 쓸 수 없습니다: %v",
		"Не удалось записать результат инструмента в %s: %v")
	add(KeyCompactResultStoreCloseResultFile,
		"persist tool result to %s: %v",
		"无法完成工具结果文件 %s 的写入：%v",
		"Das Schreiben des Tool-Ergebnisses nach %s konnte nicht abgeschlossen werden: %v",
		"ツール結果ファイル %s への書き込みを完了できませんでした：%v",
		"도구 결과 파일 %s 쓰기를 완료할 수 없습니다: %v",
		"Не удалось завершить запись результата инструмента в %s: %v")
	add(KeyCompactResultStoreSerializeStructured,
		"serialize structured tool result %s: %v",
		"无法序列化结构化工具结果 %s：%v",
		"Das strukturierte Tool-Ergebnis %s konnte nicht serialisiert werden: %v",
		"構造化されたツール結果 %s をシリアライズできませんでした：%v",
		"구조화된 도구 결과 %s을(를) 직렬화할 수 없습니다: %v",
		"Не удалось сериализовать структурированный результат инструмента %s: %v")
}
