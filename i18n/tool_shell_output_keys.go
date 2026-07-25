package i18n

const KeyToolShellImagePlaceholder Key = "tool.shell.output.image_placeholder"

func init() {
	semanticTranslations[KeyToolShellImagePlaceholder] = map[Language]string{
		LangEN: "[image: %s, %d bytes base64]",
		LangZH: "[图片：%s，%d 字节 base64]",
		LangDE: "[Bild: %s, %d Byte Base64]",
		LangJA: "[画像: %s、%d バイト base64]",
		LangKO: "[이미지: %s, %d바이트 base64]",
		LangRU: "[изображение: %s, %d байт base64]",
	}
}
