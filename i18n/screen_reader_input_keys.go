package i18n

const (
	KeyScreenReaderPollInputFailed       Key = "screen_reader.input.poll_failed"
	KeyScreenReaderReadInputFailed       Key = "screen_reader.input.read_failed"
	KeyScreenReaderDuplicateHandleFailed Key = "screen_reader.input.duplicate_handle_failed"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyScreenReaderPollInputFailed, "Could not poll screen-reader input", "无法轮询屏幕阅读器输入", "Screenreader-Eingabe konnte nicht abgefragt werden", "スクリーンリーダー入力をポーリングできませんでした", "스크린 리더 입력을 폴링할 수 없습니다", "Не удалось опросить ввод экранного диктора")
	add(KeyScreenReaderReadInputFailed, "Could not read screen-reader input", "无法读取屏幕阅读器输入", "Screenreader-Eingabe konnte nicht gelesen werden", "スクリーンリーダー入力を読み取れませんでした", "스크린 리더 입력을 읽을 수 없습니다", "Не удалось прочитать ввод экранного диктора")
	add(KeyScreenReaderDuplicateHandleFailed, "Could not duplicate the screen-reader input thread handle", "无法复制屏幕阅读器输入线程句柄", "Handle des Screenreader-Eingabethreads konnte nicht dupliziert werden", "スクリーンリーダー入力スレッドのハンドルを複製できませんでした", "스크린 리더 입력 스레드 핸들을 복제할 수 없습니다", "Не удалось дублировать дескриптор потока ввода экранного диктора")
}
