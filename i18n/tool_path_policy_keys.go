package i18n

const KeyToolPathOutsideAllowed Key = "tool.path.outside_allowed"

func init() {
	semanticTranslations[KeyToolPathOutsideAllowed] = map[Language]string{
		LangEN: "path %q is outside allowed directories",
		LangZH: "路径 %q 位于允许访问的目录之外",
		LangDE: "Pfad %q liegt außerhalb der erlaubten Verzeichnisse",
		LangJA: "パス %q は許可されたディレクトリの外にあります",
		LangKO: "경로 %q이(가) 허용된 디렉터리 밖에 있습니다",
		LangRU: "Путь %q находится вне разрешённых каталогов",
	}
}
