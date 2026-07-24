package i18n

const (
	KeySDKSessionsHomeUnavailable  Key = "sdk.sessions.home_unavailable"
	KeySDKSessionsIDRequired       Key = "sdk.sessions.id_required"
	KeySDKSessionsIDTooLong        Key = "sdk.sessions.id_too_long"
	KeySDKSessionsIDInvalid        Key = "sdk.sessions.id_invalid"
	KeySDKSessionsListFailed       Key = "sdk.sessions.list_failed"
	KeySDKSessionsPathEscapes      Key = "sdk.sessions.path_escapes"
	KeySDKSessionsNotFound         Key = "sdk.sessions.not_found"
	KeySDKSessionsStatFailed       Key = "sdk.sessions.stat_failed"
	KeySDKSessionsDeleteFailed     Key = "sdk.sessions.delete_failed"
	KeySDKSessionsDecodeEntry      Key = "sdk.sessions.decode_entry"
	KeySDKPermissionMarshalRequest Key = "sdk.permission.marshal_request"
	KeySDKPermissionSendRequest    Key = "sdk.permission.send_request"
)

var sdkResidualKeys = [...]Key{
	KeySDKSessionsHomeUnavailable,
	KeySDKSessionsIDRequired,
	KeySDKSessionsIDTooLong,
	KeySDKSessionsIDInvalid,
	KeySDKSessionsListFailed,
	KeySDKSessionsPathEscapes,
	KeySDKSessionsNotFound,
	KeySDKSessionsStatFailed,
	KeySDKSessionsDeleteFailed,
	KeySDKSessionsDecodeEntry,
	KeySDKPermissionMarshalRequest,
	KeySDKPermissionSendRequest,
}

func init() {
	addSDKResidual := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}

	addSDKResidual(KeySDKSessionsHomeUnavailable,
		"sdk/sessions: cannot determine home directory",
		"SDK 会话：无法确定用户主目录",
		"SDK-Sitzungen: Das Benutzerverzeichnis konnte nicht ermittelt werden",
		"SDK セッション: ホームディレクトリを特定できません",
		"SDK 세션: 홈 디렉터리를 확인할 수 없습니다",
		"Сеансы SDK: не удалось определить домашний каталог")
	addSDKResidual(KeySDKSessionsIDRequired,
		"sdk/sessions: session ID must not be empty",
		"SDK 会话：会话 ID 不能为空",
		"SDK-Sitzungen: Die Sitzungs-ID darf nicht leer sein",
		"SDK セッション: セッション ID は空にできません",
		"SDK 세션: 세션 ID는 비워 둘 수 없습니다",
		"Сеансы SDK: идентификатор сеанса не может быть пустым")
	addSDKResidual(KeySDKSessionsIDTooLong,
		"sdk/sessions: session ID too long (max %d chars)",
		"SDK 会话：会话 ID 过长（最多 %d 个字符）",
		"SDK-Sitzungen: Die Sitzungs-ID ist zu lang (höchstens %d Zeichen)",
		"SDK セッション: セッション ID が長すぎます（最大 %d 文字）",
		"SDK 세션: 세션 ID가 너무 깁니다(최대 %d자)",
		"Сеансы SDK: идентификатор сеанса слишком длинный (не более %d символов)")
	addSDKResidual(KeySDKSessionsIDInvalid,
		"sdk/sessions: session ID %q contains invalid characters (only alphanumeric, hyphen, underscore allowed)",
		"SDK 会话：会话 ID %q 含有无效字符（仅允许字母、数字、连字符和下划线）",
		"SDK-Sitzungen: Die Sitzungs-ID %q enthält ungültige Zeichen (zulässig sind nur Buchstaben, Ziffern, Bindestriche und Unterstriche)",
		"SDK セッション: セッション ID %q に無効な文字が含まれています（英数字、ハイフン、アンダースコアのみ使用できます）",
		"SDK 세션: 세션 ID %q에 잘못된 문자가 있습니다(영문자, 숫자, 하이픈, 밑줄만 허용)",
		"Сеансы SDK: идентификатор сеанса %q содержит недопустимые символы (разрешены только буквы, цифры, дефис и подчёркивание)")
	addSDKResidual(KeySDKSessionsListFailed,
		"sdk/sessions: list sessions: %v",
		"SDK 会话：无法列出会话：%v",
		"SDK-Sitzungen: Sitzungen konnten nicht aufgelistet werden: %v",
		"SDK セッション: セッションの一覧を取得できませんでした: %v",
		"SDK 세션: 세션 목록을 불러올 수 없습니다: %v",
		"Сеансы SDK: не удалось получить список сеансов: %v")
	addSDKResidual(KeySDKSessionsPathEscapes,
		"sdk/sessions: session path escapes sessions directory",
		"SDK 会话：会话路径超出了会话目录",
		"SDK-Sitzungen: Der Sitzungspfad liegt außerhalb des Sitzungsverzeichnisses",
		"SDK セッション: セッションのパスがセッションディレクトリの外を指しています",
		"SDK 세션: 세션 경로가 세션 디렉터리를 벗어납니다",
		"Сеансы SDK: путь к сеансу выходит за пределы каталога сеансов")
	addSDKResidual(KeySDKSessionsNotFound,
		"sdk/sessions: session %q not found",
		"SDK 会话：未找到会话 %q",
		"SDK-Sitzungen: Sitzung %q wurde nicht gefunden",
		"SDK セッション: セッション %q が見つかりません",
		"SDK 세션: 세션 %q을(를) 찾을 수 없습니다",
		"Сеансы SDK: сеанс %q не найден")
	addSDKResidual(KeySDKSessionsStatFailed,
		"sdk/sessions: stat session: %v",
		"SDK 会话：无法读取会话文件信息：%v",
		"SDK-Sitzungen: Dateiinformationen der Sitzung konnten nicht gelesen werden: %v",
		"SDK セッション: セッションファイルの情報を取得できませんでした: %v",
		"SDK 세션: 세션 파일 정보를 읽을 수 없습니다: %v",
		"Сеансы SDK: не удалось получить сведения о файле сеанса: %v")
	addSDKResidual(KeySDKSessionsDeleteFailed,
		"sdk/sessions: delete session: %v",
		"SDK 会话：无法删除会话：%v",
		"SDK-Sitzungen: Sitzung konnte nicht gelöscht werden: %v",
		"SDK セッション: セッションを削除できませんでした: %v",
		"SDK 세션: 세션을 삭제할 수 없습니다: %v",
		"Сеансы SDK: не удалось удалить сеанс: %v")
	addSDKResidual(KeySDKSessionsDecodeEntry,
		"decode entry %d: %v",
		"无法解码第 %d 条记录：%v",
		"Eintrag %d konnte nicht dekodiert werden: %v",
		"%d 件目のエントリをデコードできませんでした: %v",
		"%d번째 항목을 디코딩할 수 없습니다: %v",
		"Не удалось декодировать запись %d: %v")
	addSDKResidual(KeySDKPermissionMarshalRequest,
		"sdk: marshal permission request: %v",
		"SDK：无法序列化权限请求：%v",
		"SDK: Berechtigungsanfrage konnte nicht serialisiert werden: %v",
		"SDK: 権限リクエストをシリアライズできませんでした: %v",
		"SDK: 권한 요청을 직렬화할 수 없습니다: %v",
		"SDK: не удалось сериализовать запрос разрешения: %v")
	addSDKResidual(KeySDKPermissionSendRequest,
		"sdk: send permission request: %v",
		"SDK：无法发送权限请求：%v",
		"SDK: Berechtigungsanfrage konnte nicht gesendet werden: %v",
		"SDK: 権限リクエストを送信できませんでした: %v",
		"SDK: 권한 요청을 보낼 수 없습니다: %v",
		"SDK: не удалось отправить запрос разрешения: %v")
}
