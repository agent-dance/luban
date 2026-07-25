package i18n

import "errors"

// semanticRuntimeError defers localization until the error is rendered. This
// keeps long-running OAuth flows aligned with the current runtime language and
// preserves errors.Is/errors.As through Unwrap.
type semanticRuntimeError struct {
	key          Key
	args         []any
	cause        error
	includeCause bool
	language     Language
	languageSet  bool
}

// SemanticErrorInfo is the stable semantic portion of an error. It is useful
// at persistence boundaries that must store a key instead of freezing the
// currently rendered language.
type SemanticErrorInfo struct {
	Key          Key
	Args         []any
	Cause        error
	IncludeCause bool
}

// DescribeSemanticError extracts semantic metadata without rendering it.
func DescribeSemanticError(err error) (SemanticErrorInfo, bool) {
	var semantic *semanticRuntimeError
	if !errors.As(err, &semantic) || semantic == nil {
		return SemanticErrorInfo{}, false
	}
	return SemanticErrorInfo{
		Key: semantic.key, Args: append([]any(nil), semantic.args...),
		Cause: semantic.cause, IncludeCause: semantic.includeCause,
	}, true
}

func (e *semanticRuntimeError) Error() string {
	if e == nil {
		return ""
	}
	if e.languageSet {
		return e.Localized(e.language)
	}
	return e.Localized(DetectOrLoadLanguage())
}

// Localized renders the semantic error in an explicitly selected language.
// Boundary adapters use this when their caller has already captured the
// active display language and must not depend on a later global lookup.
func (e *semanticRuntimeError) Localized(lang Language) string {
	if e == nil {
		return ""
	}
	args := append([]any(nil), e.args...)
	if e.cause != nil && e.includeCause {
		args = append(args, e.cause)
	}
	return Format(lang, e.key, args...)
}

func (e *semanticRuntimeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// NewError creates a semantic, runtime-localized error without an underlying
// cause. Raw remote values should be passed as formatting arguments.
func NewError(key Key, args ...any) error {
	return &semanticRuntimeError{key: key, args: append([]any(nil), args...)}
}

// WrapError creates a semantic, runtime-localized error that preserves cause
// for errors.Is/errors.As. The cause is supplied as the final format argument.
func WrapError(key Key, cause error, args ...any) error {
	if cause == nil {
		return NewError(key, args...)
	}
	return &semanticRuntimeError{key: key, args: append([]any(nil), args...), cause: cause, includeCause: true}
}

// WrapInternalError preserves an internal cause for errors.Is/errors.As while
// keeping diagnostic-only English out of user-visible copy. The semantic key
// must not contain a formatting directive for the hidden cause; raw external
// errors that users need to inspect should continue to use WrapError instead.
func WrapInternalError(key Key, cause error, args ...any) error {
	if cause == nil {
		return NewError(key, args...)
	}
	return &semanticRuntimeError{key: key, args: append([]any(nil), args...), cause: cause}
}

// WrapInternalErrorInLanguage is the explicit-language form of
// WrapInternalError. Use it at UI boundaries that already captured the active
// language and must return an ordinary error without exposing an internal
// diagnostic cause.
func WrapInternalErrorInLanguage(lang Language, key Key, cause error, args ...any) error {
	if cause == nil {
		return &semanticRuntimeError{
			key: key, args: append([]any(nil), args...), language: lang, languageSet: true,
		}
	}
	return &semanticRuntimeError{
		key: key, args: append([]any(nil), args...), cause: cause,
		language: lang, languageSet: true,
	}
}

const (
	KeyAuthOAuthGenerateVerifier         Key = "auth.oauth.generate_verifier"
	KeyAuthOAuthInvalidAuthorizationURL  Key = "auth.oauth.invalid_authorization_url"
	KeyAuthOAuthGenerateState            Key = "auth.oauth.generate_state"
	KeyAuthOAuthStartCallbackServer      Key = "auth.oauth.start_callback_server"
	KeyAuthOAuthCallbackStateMismatch    Key = "auth.oauth.callback_state_mismatch"
	KeyAuthOAuthAuthorizationError       Key = "auth.oauth.authorization_error"
	KeyAuthOAuthCallbackMissingCode      Key = "auth.oauth.callback_missing_code"
	KeyAuthOAuthFlowCancelled            Key = "auth.oauth.flow_cancelled"
	KeyAuthOAuthBuildTokenRequest        Key = "auth.oauth.build_token_request"
	KeyAuthOAuthTokenRequest             Key = "auth.oauth.token_request"
	KeyAuthOAuthTokenEndpointRejected    Key = "auth.oauth.token_endpoint_rejected"
	KeyAuthOAuthDecodeTokenResponse      Key = "auth.oauth.decode_token_response"
	KeyAuthOAuthMissingTokenResponse     Key = "auth.oauth.missing_token_response"
	KeyAuthOAuthTokenMissingAccessToken  Key = "auth.oauth.token_missing_access_token"
	KeyAuthOAuthSaveRefreshedCredentials Key = "auth.oauth.save_refreshed_credentials"
	KeyAuthOAuthRefreshTokenRequired     Key = "auth.oauth.refresh_token_required"
	KeyAuthOAuthEncodeRefreshRequest     Key = "auth.oauth.encode_refresh_request"
	KeyAuthOAuthBuildRefreshRequest      Key = "auth.oauth.build_refresh_request"
	KeyAuthOAuthRefreshRequest           Key = "auth.oauth.refresh_request"
	KeyAuthOAuthRefreshEndpointRejected  Key = "auth.oauth.refresh_endpoint_rejected"
	KeyAuthOAuthDecodeRefreshResponse    Key = "auth.oauth.decode_refresh_response"
	KeyAuthOAuthChatGPTAccessTokenNeeded Key = "auth.oauth.chatgpt_access_token_needed"
	KeyAuthOAuthRefreshFailed            Key = "auth.oauth.refresh_failed"
	KeyAuthOAuthNoCredentials            Key = "auth.oauth.no_credentials"
	KeyAuthOAuthCredentialsExpired       Key = "auth.oauth.credentials_expired"

	KeyAuthDeviceCancelled             Key = "auth.device.cancelled"
	KeyAuthDeviceCodeExpiredAfter      Key = "auth.device.code_expired_after"
	KeyAuthDeviceCodeExpired           Key = "auth.device.code_expired"
	KeyAuthDeviceAuthorizationDenied   Key = "auth.device.authorization_denied"
	KeyAuthDeviceBuildCodeRequest      Key = "auth.device.build_code_request"
	KeyAuthDeviceCodeRequest           Key = "auth.device.code_request"
	KeyAuthDeviceCodeEndpointRejected  Key = "auth.device.code_endpoint_rejected"
	KeyAuthDeviceDecodeCodeResponse    Key = "auth.device.decode_code_response"
	KeyAuthDeviceBuildTokenRequest     Key = "auth.device.build_token_request"
	KeyAuthDeviceTokenRequest          Key = "auth.device.token_request"
	KeyAuthDeviceReadTokenResponse     Key = "auth.device.read_token_response"
	KeyAuthDeviceTokenEndpointRejected Key = "auth.device.token_endpoint_rejected"
	KeyAuthDeviceDecodeTokenResponse   Key = "auth.device.decode_token_response"
	KeyAuthDeviceRemoteError           Key = "auth.device.remote_error"
	KeyAuthDeviceRemoteErrorDetail     Key = "auth.device.remote_error_detail"

	KeyAuthStoreAcquireLock        Key = "auth.store.acquire_lock"
	KeyAuthStoreLockHeld           Key = "auth.store.lock_held"
	KeyAuthStoreHomeUnavailable    Key = "auth.store.home_unavailable"
	KeyAuthStoreCreateDirectory    Key = "auth.store.create_directory"
	KeyAuthStoreRead               Key = "auth.store.read"
	KeyAuthStoreDecode             Key = "auth.store.decode"
	KeyAuthStoreEncode             Key = "auth.store.encode"
	KeyAuthStoreCreateTemporary    Key = "auth.store.create_temporary"
	KeyAuthStoreWriteTemporary     Key = "auth.store.write_temporary"
	KeyAuthStoreSetPermissions     Key = "auth.store.set_permissions"
	KeyAuthStoreCloseTemporary     Key = "auth.store.close_temporary"
	KeyAuthStoreReplaceCredentials Key = "auth.store.replace_credentials"
)

var authRuntimeErrorKeys = []Key{
	KeyAuthOAuthGenerateVerifier, KeyAuthOAuthInvalidAuthorizationURL,
	KeyAuthOAuthGenerateState, KeyAuthOAuthStartCallbackServer,
	KeyAuthOAuthCallbackStateMismatch, KeyAuthOAuthAuthorizationError,
	KeyAuthOAuthCallbackMissingCode, KeyAuthOAuthFlowCancelled,
	KeyAuthOAuthBuildTokenRequest, KeyAuthOAuthTokenRequest,
	KeyAuthOAuthTokenEndpointRejected, KeyAuthOAuthDecodeTokenResponse,
	KeyAuthOAuthMissingTokenResponse, KeyAuthOAuthTokenMissingAccessToken,
	KeyAuthOAuthSaveRefreshedCredentials, KeyAuthOAuthRefreshTokenRequired,
	KeyAuthOAuthEncodeRefreshRequest, KeyAuthOAuthBuildRefreshRequest,
	KeyAuthOAuthRefreshRequest, KeyAuthOAuthRefreshEndpointRejected,
	KeyAuthOAuthDecodeRefreshResponse, KeyAuthOAuthChatGPTAccessTokenNeeded,
	KeyAuthOAuthRefreshFailed, KeyAuthOAuthNoCredentials,
	KeyAuthOAuthCredentialsExpired,
	KeyAuthDeviceCancelled, KeyAuthDeviceCodeExpiredAfter,
	KeyAuthDeviceCodeExpired, KeyAuthDeviceAuthorizationDenied,
	KeyAuthDeviceBuildCodeRequest, KeyAuthDeviceCodeRequest,
	KeyAuthDeviceCodeEndpointRejected, KeyAuthDeviceDecodeCodeResponse,
	KeyAuthDeviceBuildTokenRequest, KeyAuthDeviceTokenRequest,
	KeyAuthDeviceReadTokenResponse, KeyAuthDeviceTokenEndpointRejected,
	KeyAuthDeviceDecodeTokenResponse, KeyAuthDeviceRemoteError,
	KeyAuthDeviceRemoteErrorDetail,
	KeyAuthStoreAcquireLock, KeyAuthStoreLockHeld, KeyAuthStoreHomeUnavailable,
	KeyAuthStoreCreateDirectory, KeyAuthStoreRead,
	KeyAuthStoreDecode, KeyAuthStoreEncode, KeyAuthStoreCreateTemporary,
	KeyAuthStoreWriteTemporary, KeyAuthStoreSetPermissions,
	KeyAuthStoreCloseTemporary, KeyAuthStoreReplaceCredentials,
}

func init() {
	addAuthRuntimeError(KeyAuthOAuthGenerateVerifier,
		"Failed to generate the OAuth code verifier: %v",
		"生成 OAuth code verifier 失败：%v",
		"Der OAuth-Code-Verifier konnte nicht erzeugt werden: %v",
		"OAuth code verifier を生成できませんでした：%v",
		"OAuth code verifier를 생성하지 못했습니다: %v",
		"Не удалось создать verifier кода OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthInvalidAuthorizationURL,
		"Invalid OAuth authorization URL %q: %v",
		"OAuth 授权 URL %q 无效：%v",
		"Ungültige OAuth-Autorisierungs-URL %q: %v",
		"OAuth 認可 URL %q が無効です：%v",
		"OAuth 인증 URL %q이(가) 올바르지 않습니다: %v",
		"Недопустимый URL авторизации OAuth %q: %v")
	addAuthRuntimeError(KeyAuthOAuthGenerateState,
		"Failed to generate the OAuth state: %v",
		"生成 OAuth state 失败：%v",
		"Der OAuth-State konnte nicht erzeugt werden: %v",
		"OAuth state を生成できませんでした：%v",
		"OAuth state를 생성하지 못했습니다: %v",
		"Не удалось создать параметр state для OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthStartCallbackServer,
		"Failed to start the OAuth callback server: %v",
		"启动 OAuth 回调服务器失败：%v",
		"Der OAuth-Callback-Server konnte nicht gestartet werden: %v",
		"OAuth コールバックサーバーを起動できませんでした：%v",
		"OAuth 콜백 서버를 시작하지 못했습니다: %v",
		"Не удалось запустить сервер обратного вызова OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthCallbackStateMismatch,
		"OAuth callback state mismatch; received %q",
		"OAuth 回调的 state 不匹配；收到 %q",
		"Der State im OAuth-Callback stimmt nicht überein; empfangen: %q",
		"OAuth コールバックの state が一致しません。受信値：%q",
		"OAuth 콜백의 state가 일치하지 않습니다. 수신값: %q",
		"Параметр state в ответе OAuth не совпадает; получено: %q")
	addAuthRuntimeError(KeyAuthOAuthAuthorizationError,
		"OAuth authorization failed with %q: %s",
		"OAuth 授权失败（%q）：%s",
		"OAuth-Autorisierung mit %q fehlgeschlagen: %s",
		"OAuth 認可が失敗しました（%q）：%s",
		"OAuth 권한 부여에 실패했습니다(%q): %s",
		"Ошибка авторизации OAuth %q: %s")
	addAuthRuntimeError(KeyAuthOAuthCallbackMissingCode,
		"The OAuth callback did not include an authorization code",
		"OAuth 回调中缺少授权码",
		"Der OAuth-Callback enthält keinen Autorisierungscode",
		"OAuth コールバックに認可コードがありません",
		"OAuth 콜백에 인증 코드가 없습니다",
		"В ответе OAuth отсутствует код авторизации")
	addAuthRuntimeError(KeyAuthOAuthFlowCancelled,
		"OAuth authorization was cancelled: %v",
		"OAuth 授权已取消：%v",
		"Die OAuth-Autorisierung wurde abgebrochen: %v",
		"OAuth 認可がキャンセルされました：%v",
		"OAuth 권한 부여가 취소되었습니다: %v",
		"Авторизация OAuth отменена: %v")
	addAuthRuntimeError(KeyAuthOAuthBuildTokenRequest,
		"Failed to build the OAuth token request: %v",
		"构建 OAuth token 请求失败：%v",
		"Die OAuth-Token-Anfrage konnte nicht erstellt werden: %v",
		"OAuth token リクエストを作成できませんでした：%v",
		"OAuth token 요청을 만들지 못했습니다: %v",
		"Не удалось сформировать запрос token OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthTokenRequest,
		"OAuth token request failed: %v",
		"OAuth token 请求失败：%v",
		"Die OAuth-Token-Anfrage ist fehlgeschlagen: %v",
		"OAuth token リクエストに失敗しました：%v",
		"OAuth token 요청에 실패했습니다: %v",
		"Запрос token OAuth завершился ошибкой: %v")
	addAuthRuntimeError(KeyAuthOAuthTokenEndpointRejected,
		"OAuth token endpoint returned HTTP %d: %s",
		"OAuth token endpoint 返回 HTTP %d：%s",
		"Der OAuth-Token-Endpunkt hat HTTP %d zurückgegeben: %s",
		"OAuth token endpoint が HTTP %d を返しました：%s",
		"OAuth token endpoint가 HTTP %d을(를) 반환했습니다: %s",
		"Endpoint token OAuth вернул HTTP %d: %s")
	addAuthRuntimeError(KeyAuthOAuthDecodeTokenResponse,
		"Failed to decode the OAuth token response: %v",
		"解析 OAuth token 响应失败：%v",
		"Die OAuth-Token-Antwort konnte nicht dekodiert werden: %v",
		"OAuth token レスポンスを解析できませんでした：%v",
		"OAuth token 응답을 디코딩하지 못했습니다: %v",
		"Не удалось декодировать ответ token OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthMissingTokenResponse,
		"The OAuth server returned no token response",
		"OAuth 服务器未返回 token 响应",
		"Der OAuth-Server hat keine Token-Antwort zurückgegeben",
		"OAuth サーバーから token レスポンスが返されませんでした",
		"OAuth 서버가 token 응답을 반환하지 않았습니다",
		"Сервер OAuth не вернул ответ с token")
	addAuthRuntimeError(KeyAuthOAuthTokenMissingAccessToken,
		"The OAuth token response did not include an access_token",
		"OAuth token 响应中缺少 access_token",
		"Die OAuth-Token-Antwort enthält kein access_token",
		"OAuth token レスポンスに access_token がありません",
		"OAuth token 응답에 access_token이 없습니다",
		"В ответе token OAuth отсутствует access_token")
	addAuthRuntimeError(KeyAuthOAuthSaveRefreshedCredentials,
		"Failed to save refreshed OAuth credentials: %v",
		"保存刷新后的 OAuth 凭据失败：%v",
		"Die erneuerten OAuth-Zugangsdaten konnten nicht gespeichert werden: %v",
		"更新した OAuth 認証情報を保存できませんでした：%v",
		"갱신된 OAuth 자격 증명을 저장하지 못했습니다: %v",
		"Не удалось сохранить обновлённые учётные данные OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthRefreshTokenRequired,
		"An OAuth refresh token is required",
		"需要 OAuth refresh token",
		"Ein OAuth-Refresh-Token ist erforderlich",
		"OAuth refresh token が必要です",
		"OAuth refresh token이 필요합니다",
		"Требуется refresh token OAuth")
	addAuthRuntimeError(KeyAuthOAuthEncodeRefreshRequest,
		"Failed to encode the OAuth refresh request: %v",
		"编码 OAuth refresh 请求失败：%v",
		"Die OAuth-Aktualisierungsanfrage konnte nicht kodiert werden: %v",
		"OAuth refresh リクエストをエンコードできませんでした：%v",
		"OAuth refresh 요청을 인코딩하지 못했습니다: %v",
		"Не удалось закодировать запрос обновления OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthBuildRefreshRequest,
		"Failed to build the OAuth refresh request: %v",
		"构建 OAuth refresh 请求失败：%v",
		"Die OAuth-Aktualisierungsanfrage konnte nicht erstellt werden: %v",
		"OAuth refresh リクエストを作成できませんでした：%v",
		"OAuth refresh 요청을 만들지 못했습니다: %v",
		"Не удалось сформировать запрос обновления OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthRefreshRequest,
		"OAuth refresh request failed: %v",
		"OAuth refresh 请求失败：%v",
		"Die OAuth-Aktualisierungsanfrage ist fehlgeschlagen: %v",
		"OAuth refresh リクエストに失敗しました：%v",
		"OAuth refresh 요청에 실패했습니다: %v",
		"Запрос обновления OAuth завершился ошибкой: %v")
	addAuthRuntimeError(KeyAuthOAuthRefreshEndpointRejected,
		"OAuth refresh endpoint returned HTTP %d: %s",
		"OAuth refresh endpoint 返回 HTTP %d：%s",
		"Der OAuth-Aktualisierungsendpunkt hat HTTP %d zurückgegeben: %s",
		"OAuth refresh endpoint が HTTP %d を返しました：%s",
		"OAuth refresh endpoint가 HTTP %d을(를) 반환했습니다: %s",
		"Endpoint обновления OAuth вернул HTTP %d: %s")
	addAuthRuntimeError(KeyAuthOAuthDecodeRefreshResponse,
		"Failed to decode the OAuth refresh response: %v",
		"解析 OAuth refresh 响应失败：%v",
		"Die OAuth-Aktualisierungsantwort konnte nicht dekodiert werden: %v",
		"OAuth refresh レスポンスを解析できませんでした：%v",
		"OAuth refresh 응답을 디코딩하지 못했습니다: %v",
		"Не удалось декодировать ответ обновления OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthChatGPTAccessTokenNeeded,
		"An access token is required for ChatGPT-backed requests",
		"使用 ChatGPT backend 发起请求时需要 access token",
		"Für Anfragen über das ChatGPT-Backend ist ein Access-Token erforderlich",
		"ChatGPT backend を使用するリクエストには access token が必要です",
		"ChatGPT backend 요청에는 access token이 필요합니다",
		"Для запросов через backend ChatGPT требуется access token")
	addAuthRuntimeError(KeyAuthOAuthRefreshFailed,
		"OAuth credential refresh failed: %v",
		"刷新 OAuth 凭据失败：%v",
		"Die OAuth-Zugangsdaten konnten nicht erneuert werden: %v",
		"OAuth 認証情報の更新に失敗しました：%v",
		"OAuth 자격 증명 갱신에 실패했습니다: %v",
		"Не удалось обновить учётные данные OAuth: %v")
	addAuthRuntimeError(KeyAuthOAuthNoCredentials,
		"No OAuth credentials were found; complete OAuth login first",
		"未找到 OAuth 凭据；请先完成 OAuth 登录",
		"Keine OAuth-Zugangsdaten gefunden; führe zuerst die OAuth-Anmeldung durch",
		"OAuth 認証情報がありません。先に OAuth ログインを完了してください",
		"OAuth 자격 증명을 찾을 수 없습니다. 먼저 OAuth 로그인을 완료하세요",
		"Учётные данные OAuth не найдены; сначала выполните вход через OAuth")
	addAuthRuntimeError(KeyAuthOAuthCredentialsExpired,
		"OAuth credentials expired and no refresh token is available",
		"OAuth 凭据已过期，且没有可用的 refresh token",
		"Die OAuth-Zugangsdaten sind abgelaufen und es ist kein Refresh-Token verfügbar",
		"OAuth 認証情報の有効期限が切れており、refresh token もありません",
		"OAuth 자격 증명이 만료되었고 사용할 수 있는 refresh token이 없습니다",
		"Учётные данные OAuth истекли, а refresh token недоступен")

	addAuthRuntimeError(KeyAuthDeviceCancelled,
		"Device Authorization was cancelled: %v",
		"Device Authorization 已取消：%v",
		"Device Authorization wurde abgebrochen: %v",
		"Device Authorization がキャンセルされました：%v",
		"Device Authorization이 취소되었습니다: %v",
		"Device Authorization отменена: %v")
	addAuthRuntimeError(KeyAuthDeviceCodeExpiredAfter,
		"The device code expired after %v",
		"device code 已在 %v 后过期",
		"Der Gerätecode ist nach %v abgelaufen",
		"device code は %v 後に期限切れになりました",
		"device code가 %v 후 만료되었습니다",
		"Срок действия device code истёк через %v")
	addAuthRuntimeError(KeyAuthDeviceCodeExpired,
		"The device code has expired",
		"device code 已过期",
		"Der Gerätecode ist abgelaufen",
		"device code の有効期限が切れました",
		"device code가 만료되었습니다",
		"Срок действия device code истёк")
	addAuthRuntimeError(KeyAuthDeviceAuthorizationDenied,
		"Device Authorization was denied",
		"Device Authorization 被拒绝",
		"Device Authorization wurde abgelehnt",
		"Device Authorization が拒否されました",
		"Device Authorization이 거부되었습니다",
		"Device Authorization отклонена")
	addAuthRuntimeError(KeyAuthDeviceBuildCodeRequest,
		"Failed to build the device-code request: %v",
		"构建 device code 请求失败：%v",
		"Die Gerätecode-Anfrage konnte nicht erstellt werden: %v",
		"device code リクエストを作成できませんでした：%v",
		"device code 요청을 만들지 못했습니다: %v",
		"Не удалось сформировать запрос device code: %v")
	addAuthRuntimeError(KeyAuthDeviceCodeRequest,
		"Device-code request failed: %v",
		"device code 请求失败：%v",
		"Die Gerätecode-Anfrage ist fehlgeschlagen: %v",
		"device code リクエストに失敗しました：%v",
		"device code 요청에 실패했습니다: %v",
		"Запрос device code завершился ошибкой: %v")
	addAuthRuntimeError(KeyAuthDeviceCodeEndpointRejected,
		"Device-code endpoint returned HTTP %d: %s",
		"device code endpoint 返回 HTTP %d：%s",
		"Der Gerätecode-Endpunkt hat HTTP %d zurückgegeben: %s",
		"device code endpoint が HTTP %d を返しました：%s",
		"device code endpoint가 HTTP %d을(를) 반환했습니다: %s",
		"Endpoint device code вернул HTTP %d: %s")
	addAuthRuntimeError(KeyAuthDeviceDecodeCodeResponse,
		"Failed to decode the device-code response: %v",
		"解析 device code 响应失败：%v",
		"Die Gerätecode-Antwort konnte nicht dekodiert werden: %v",
		"device code レスポンスを解析できませんでした：%v",
		"device code 응답을 디코딩하지 못했습니다: %v",
		"Не удалось декодировать ответ device code: %v")
	addAuthRuntimeError(KeyAuthDeviceBuildTokenRequest,
		"Failed to build the device-token request: %v",
		"构建 device token 请求失败：%v",
		"Die Geräte-Token-Anfrage konnte nicht erstellt werden: %v",
		"device token リクエストを作成できませんでした：%v",
		"device token 요청을 만들지 못했습니다: %v",
		"Не удалось сформировать запрос device token: %v")
	addAuthRuntimeError(KeyAuthDeviceTokenRequest,
		"Device-token request failed: %v",
		"device token 请求失败：%v",
		"Die Geräte-Token-Anfrage ist fehlgeschlagen: %v",
		"device token リクエストに失敗しました：%v",
		"device token 요청에 실패했습니다: %v",
		"Запрос device token завершился ошибкой: %v")
	addAuthRuntimeError(KeyAuthDeviceReadTokenResponse,
		"Failed to read the device-token response: %v",
		"读取 device token 响应失败：%v",
		"Die Geräte-Token-Antwort konnte nicht gelesen werden: %v",
		"device token レスポンスを読み取れませんでした：%v",
		"device token 응답을 읽지 못했습니다: %v",
		"Не удалось прочитать ответ device token: %v")
	addAuthRuntimeError(KeyAuthDeviceTokenEndpointRejected,
		"Device-token endpoint returned HTTP %d: %s",
		"device token endpoint 返回 HTTP %d：%s",
		"Der Geräte-Token-Endpunkt hat HTTP %d zurückgegeben: %s",
		"device token endpoint が HTTP %d を返しました：%s",
		"device token endpoint가 HTTP %d을(를) 반환했습니다: %s",
		"Endpoint device token вернул HTTP %d: %s")
	addAuthRuntimeError(KeyAuthDeviceDecodeTokenResponse,
		"Failed to decode the device-token response: %v",
		"解析 device token 响应失败：%v",
		"Die Geräte-Token-Antwort konnte nicht dekodiert werden: %v",
		"device token レスポンスを解析できませんでした：%v",
		"device token 응답을 디코딩하지 못했습니다: %v",
		"Не удалось декодировать ответ device token: %v")
	addAuthRuntimeError(KeyAuthDeviceRemoteError,
		"Device Authorization error %s",
		"Device Authorization 错误：%s",
		"Device-Authorization-Fehler %s",
		"Device Authorization エラー：%s",
		"Device Authorization 오류: %s",
		"Ошибка Device Authorization: %s")
	addAuthRuntimeError(KeyAuthDeviceRemoteErrorDetail,
		"Device Authorization error %s: %s",
		"Device Authorization 错误（%s）：%s",
		"Device-Authorization-Fehler %s: %s",
		"Device Authorization エラー（%s）：%s",
		"Device Authorization 오류(%s): %s",
		"Ошибка Device Authorization %s: %s")

	addAuthRuntimeError(KeyAuthStoreAcquireLock,
		"Failed to acquire the OAuth credential lock: %v",
		"获取 OAuth 凭据锁失败：%v",
		"Die Sperre für OAuth-Zugangsdaten konnte nicht gesetzt werden: %v",
		"OAuth 認証情報のロックを取得できませんでした：%v",
		"OAuth 자격 증명 잠금을 획득하지 못했습니다: %v",
		"Не удалось установить блокировку учётных данных OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreLockHeld,
		"The OAuth credential lock is still held after %d attempts; another process may be stuck",
		"尝试 %d 次后 OAuth 凭据锁仍被占用；另一个进程可能已卡住",
		"Die Sperre für OAuth-Zugangsdaten ist nach %d Versuchen noch belegt; möglicherweise hängt ein anderer Prozess",
		"%d 回試行しても OAuth 認証情報のロックが保持されています。別のプロセスが停止している可能性があります",
		"%d번 시도한 후에도 OAuth 자격 증명 잠금이 유지되고 있습니다. 다른 프로세스가 멈췄을 수 있습니다",
		"Блокировка учётных данных OAuth не снята после %d попыток; возможно, другой процесс завис")
	addAuthRuntimeError(KeyAuthStoreHomeUnavailable,
		"The home directory for OAuth credentials is unavailable",
		"无法确定 OAuth 凭据的主目录",
		"Das Home-Verzeichnis für OAuth-Zugangsdaten ist nicht verfügbar",
		"OAuth 認証情報用のホームディレクトリを特定できません",
		"OAuth 자격 증명용 홈 디렉터리를 확인할 수 없습니다",
		"Не удалось определить домашний каталог для учётных данных OAuth")
	addAuthRuntimeError(KeyAuthStoreCreateDirectory,
		"Failed to create the OAuth credential directory: %v",
		"创建 OAuth 凭据目录失败：%v",
		"Das Verzeichnis für OAuth-Zugangsdaten konnte nicht erstellt werden: %v",
		"OAuth 認証情報ディレクトリを作成できませんでした：%v",
		"OAuth 자격 증명 디렉터리를 만들지 못했습니다: %v",
		"Не удалось создать каталог учётных данных OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreRead,
		"Failed to read OAuth credentials: %v",
		"读取 OAuth 凭据失败：%v",
		"Die OAuth-Zugangsdaten konnten nicht gelesen werden: %v",
		"OAuth 認証情報を読み取れませんでした：%v",
		"OAuth 자격 증명을 읽지 못했습니다: %v",
		"Не удалось прочитать учётные данные OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreDecode,
		"Failed to decode OAuth credentials: %v",
		"解析 OAuth 凭据失败：%v",
		"Die OAuth-Zugangsdaten konnten nicht dekodiert werden: %v",
		"OAuth 認証情報を解析できませんでした：%v",
		"OAuth 자격 증명을 디코딩하지 못했습니다: %v",
		"Не удалось декодировать учётные данные OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreEncode,
		"Failed to encode OAuth credentials: %v",
		"编码 OAuth 凭据失败：%v",
		"Die OAuth-Zugangsdaten konnten nicht kodiert werden: %v",
		"OAuth 認証情報をエンコードできませんでした：%v",
		"OAuth 자격 증명을 인코딩하지 못했습니다: %v",
		"Не удалось закодировать учётные данные OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreCreateTemporary,
		"Failed to create a temporary OAuth credential file: %v",
		"创建 OAuth 凭据临时文件失败：%v",
		"Die temporäre Datei für OAuth-Zugangsdaten konnte nicht erstellt werden: %v",
		"OAuth 認証情報の一時ファイルを作成できませんでした：%v",
		"OAuth 자격 증명 임시 파일을 만들지 못했습니다: %v",
		"Не удалось создать временный файл учётных данных OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreWriteTemporary,
		"Failed to write the temporary OAuth credential file: %v",
		"写入 OAuth 凭据临时文件失败：%v",
		"Die temporäre Datei für OAuth-Zugangsdaten konnte nicht geschrieben werden: %v",
		"OAuth 認証情報の一時ファイルを書き込めませんでした：%v",
		"OAuth 자격 증명 임시 파일을 쓰지 못했습니다: %v",
		"Не удалось записать временный файл учётных данных OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreSetPermissions,
		"Failed to restrict OAuth credential file permissions: %v",
		"限制 OAuth 凭据文件权限失败：%v",
		"Die Dateirechte der OAuth-Zugangsdaten konnten nicht eingeschränkt werden: %v",
		"OAuth 認証情報ファイルの権限を制限できませんでした：%v",
		"OAuth 자격 증명 파일 권한을 제한하지 못했습니다: %v",
		"Не удалось ограничить права на файл учётных данных OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreCloseTemporary,
		"Failed to close the temporary OAuth credential file: %v",
		"关闭 OAuth 凭据临时文件失败：%v",
		"Die temporäre Datei für OAuth-Zugangsdaten konnte nicht geschlossen werden: %v",
		"OAuth 認証情報の一時ファイルを閉じられませんでした：%v",
		"OAuth 자격 증명 임시 파일을 닫지 못했습니다: %v",
		"Не удалось закрыть временный файл учётных данных OAuth: %v")
	addAuthRuntimeError(KeyAuthStoreReplaceCredentials,
		"Failed to replace the OAuth credential file: %v",
		"替换 OAuth 凭据文件失败：%v",
		"Die Datei mit OAuth-Zugangsdaten konnte nicht ersetzt werden: %v",
		"OAuth 認証情報ファイルを置き換えられませんでした：%v",
		"OAuth 자격 증명 파일을 교체하지 못했습니다: %v",
		"Не удалось заменить файл учётных данных OAuth: %v")
}

func addAuthRuntimeError(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
