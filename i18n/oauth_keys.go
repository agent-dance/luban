package i18n

const (
	KeyOAuthInvalidState          Key = "oauth.callback.invalid_state"
	KeyOAuthAuthorizationDenied   Key = "oauth.callback.authorization_denied"
	KeyOAuthMissingCode           Key = "oauth.callback.missing_code"
	KeyOAuthAuthorizationSuccess  Key = "oauth.callback.authorization_success"
	KeyOAuthAuthenticationError   Key = "oauth.callback.authentication_error"
	KeyOAuthAuthenticationSuccess Key = "oauth.callback.authentication_success"
)

func init() {
	addOAuth(KeyOAuthInvalidState, "Invalid state parameter. You may close this window.", "state 参数无效。你可以关闭此窗口。", "Ungültiger state-Parameter. Du kannst dieses Fenster schließen.", "state パラメータが無効です。このウィンドウを閉じてもかまいません。", "state 매개변수가 올바르지 않습니다. 이 창을 닫아도 됩니다.", "Недопустимый параметр state. Это окно можно закрыть.")
	addOAuth(KeyOAuthAuthorizationDenied, "Authorization was denied: %s. You may close this window.", "授权被拒绝：%s。你可以关闭此窗口。", "Autorisierung abgelehnt: %s. Du kannst dieses Fenster schließen.", "認可が拒否されました: %s。このウィンドウを閉じてもかまいません。", "권한 부여가 거부되었습니다: %s. 이 창을 닫아도 됩니다.", "Авторизация отклонена: %s. Это окно можно закрыть.")
	addOAuth(KeyOAuthMissingCode, "The authorization code is missing. You may close this window.", "缺少授权码。你可以关闭此窗口。", "Der Autorisierungscode fehlt. Du kannst dieses Fenster schließen.", "認可コードがありません。このウィンドウを閉じてもかまいません。", "인증 코드가 없습니다. 이 창을 닫아도 됩니다.", "Отсутствует код авторизации. Это окно можно закрыть.")
	addOAuth(KeyOAuthAuthorizationSuccess, "Authorization successful. You may close this window.", "授权成功。你可以关闭此窗口。", "Autorisierung erfolgreich. Du kannst dieses Fenster schließen.", "認可が完了しました。このウィンドウを閉じてもかまいません。", "권한 부여가 완료되었습니다. 이 창을 닫아도 됩니다.", "Авторизация выполнена. Это окно можно закрыть.")
	addOAuth(KeyOAuthAuthenticationError, "Authentication failed. You may close this window.", "认证失败。你可以关闭此窗口。", "Authentifizierung fehlgeschlagen. Du kannst dieses Fenster schließen.", "認証に失敗しました。このウィンドウを閉じてもかまいません。", "인증에 실패했습니다. 이 창을 닫아도 됩니다.", "Аутентификация не удалась. Это окно можно закрыть.")
	addOAuth(KeyOAuthAuthenticationSuccess, "Authentication successful. You may close this window.", "认证成功。你可以关闭此窗口。", "Authentifizierung erfolgreich. Du kannst dieses Fenster schließen.", "認証が完了しました。このウィンドウを閉じてもかまいません。", "인증이 완료되었습니다. 이 창을 닫아도 됩니다.", "Аутентификация выполнена. Это окно можно закрыть.")
}

func addOAuth(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
