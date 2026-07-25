package i18n

const (
	KeySDKSessionsIDRequired       Key = "sdk.sessions.id_required"
	KeySDKSessionsIDTooLong        Key = "sdk.sessions.id_too_long"
	KeySDKSessionsIDInvalid        Key = "sdk.sessions.id_invalid"
	KeySDKSessionsListFailed       Key = "sdk.sessions.list_failed"
	KeySDKPermissionMarshalRequest Key = "sdk.permission.marshal_request"
	KeySDKPermissionSendRequest    Key = "sdk.permission.send_request"
)

var sdkResidualKeys = [...]Key{
	KeySDKSessionsIDRequired,
	KeySDKSessionsIDTooLong,
	KeySDKSessionsIDInvalid,
	KeySDKSessionsListFailed,
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
