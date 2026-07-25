package i18n

import "testing"

func TestProviderCompatibleKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyProviderCompatibleBaseURLRequired, KeyProviderCompatibleBaseURLInvalid,
		KeyProviderCompatibleCatalogUnavailable, KeyProviderCompatibleModelsRequestBuildFailed,
		KeyProviderCompatibleModelsRequestFailed, KeyProviderCompatibleModelsReadFailed,
		KeyProviderCompatibleModelsHTTPFailed, KeyProviderCompatibleModelsDecodeFailed,
		KeyProviderCompatibleModelsEmpty,
		KeyProviderPickerActionsCreate, KeyProviderPickerActionsConnectedCustom,
		KeyProviderPickerActionsConfigureCustom, KeyProviderPickerOther,
		KeyProviderConnectAPIStyle, KeyProviderConnectName, KeyProviderConnectNameDefault,
		KeyProviderDeleteTitle, KeyProviderDeleteWarning, KeyProviderDeleteConfirm,
		KeyREPLTUIFetchingModels, KeyREPLTUIProviderDeleted,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
