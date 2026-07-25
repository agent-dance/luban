// Package i18n provides multi-language support for the LUBAN Code UI.
// It includes translations for English, Chinese, German, Japanese, Korean, and Russian.
package i18n

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/brand"
)

// Language represents a supported language.
type Language int

var detectedLanguageCache atomic.Int32

func init() {
	detectedLanguageCache.Store(-1)
}

const (
	LangEN Language = iota // English
	LangZH                 // Chinese (Simplified)
	LangDE                 // German
	LangJA                 // Japanese
	LangKO                 // Korean
	LangRU                 // Russian
)

// String returns the display name of the language.
func (l Language) String() string {
	switch l {
	case LangEN:
		return "English"
	case LangZH:
		return "中文"
	case LangDE:
		return "Deutsch"
	case LangJA:
		return "日本語"
	case LangKO:
		return "한국어"
	case LangRU:
		return "Русский"
	default:
		return "English"
	}
}

// Code returns the ISO 639-1 language code.
func (l Language) Code() string {
	switch l {
	case LangEN:
		return "en"
	case LangZH:
		return "zh"
	case LangDE:
		return "de"
	case LangJA:
		return "ja"
	case LangKO:
		return "ko"
	case LangRU:
		return "ru"
	default:
		return "en"
	}
}

// Next returns the next language in the cycle.
func (l Language) Next() Language {
	switch l {
	case LangEN:
		return LangZH
	case LangZH:
		return LangDE
	case LangDE:
		return LangJA
	case LangJA:
		return LangKO
	case LangKO:
		return LangRU
	case LangRU:
		return LangEN
	default:
		return LangEN
	}
}

// AllLanguages returns all supported languages.
func AllLanguages() []Language {
	return []Language{LangEN, LangZH, LangDE, LangJA, LangKO, LangRU}
}

// DetectLanguage detects the system language by checking:
// 1. The LANG environment variable
// 2. The LC_ALL / LC_MESSAGES environment variables
// 3. The OS locale command output
// 4. Falls back to timezone-based detection
// 5. Defaults to English
func DetectLanguage() Language {
	// Try LANG env var
	if lang := os.Getenv("LANG"); lang != "" {
		if l := parseLangCode(lang); l != nil {
			return *l
		}
	}

	// Try LC_ALL and LC_MESSAGES
	for _, env := range []string{"LC_ALL", "LC_MESSAGES"} {
		if v := os.Getenv(env); v != "" {
			if l := parseLangCode(v); l != nil {
				return *l
			}
		}
	}

	// Try locale command on Unix
	if runtime.GOOS != "windows" {
		cmd := exec.Command("locale", "-a")
		if output, err := cmd.Output(); err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if l := parseLangCode(line); l != nil {
					return *l
				}
			}
		}
	}

	// Fallback: detect by timezone
	return detectLanguageByTimezone()
}

func parseLangCode(s string) *Language {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Handle formats like "en_US.UTF-8", "zh_CN", "de_DE@euro", "ja_JP"
	// Also handle plain codes like "en", "zh"
	lang := strings.Split(s, ".")[0]
	lang = strings.Split(lang, "@")[0]
	lang = strings.Split(lang, "_")[0]
	lang = strings.ToLower(lang)

	var l Language
	switch lang {
	case "en":
		l = LangEN
	case "zh", "cmn":
		l = LangZH
	case "de":
		l = LangDE
	case "ja":
		l = LangJA
	case "ko":
		l = LangKO
	case "ru":
		l = LangRU
	default:
		return nil
	}
	return &l
}

func detectLanguageByTimezone() Language {
	now := time.Now()
	name, offset := now.Zone()
	name = strings.ToLower(name)

	// Timezone region detection
	// Asia timezones
	asianZones := []string{"cst", "asia", "shanghai", "beijing", "chongqing", "hongkong", "taipei", "singapore"}
	for _, z := range asianZones {
		if strings.Contains(name, z) {
			// Distinguish Chinese from Japanese/Korean
			// China is UTC+8, Japan is UTC+9, Korea is UTC+9
			switch {
			case strings.Contains(name, "japan") || strings.Contains(name, "jst") || strings.Contains(name, "tokyo"):
				return LangJA
			case strings.Contains(name, "korea") || strings.Contains(name, "kst") || strings.Contains(name, "seoul"):
				return LangKO
			case offset == 28800: // UTC+8
				return LangZH
			case offset == 32400: // UTC+9
				// Could be Japan or Korea, default to Japanese
				return LangJA
			}
		}
	}

	// European timezones
	if strings.Contains(name, "cet") || strings.Contains(name, "eet") ||
		strings.Contains(name, "europe/berlin") || strings.Contains(name, "europe/vienna") ||
		strings.Contains(name, "europe/zurich") {
		return LangDE
	}
	if strings.Contains(name, "eet") || strings.Contains(name, "europe") ||
		strings.Contains(name, "msk") || strings.Contains(name, "moscow") {
		return LangRU
	}

	// Japanese timezone directly
	if strings.Contains(name, "tokyo") || strings.Contains(name, "jst") {
		return LangJA
	}

	// Korean timezone directly
	if strings.Contains(name, "seoul") || strings.Contains(name, "kst") {
		return LangKO
	}

	return LangEN
}

// LanguageFile returns the path to the language preference file.
func LanguageFile() string {
	return filepath.Join(brand.UserConfigDir(), "language.json")
}

// SaveLanguage persists the language preference to disk.
func SaveLanguage(lang Language) error {
	dir := brand.UserConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"language": lang.Code()})
	if err != nil {
		return err
	}
	if err := os.WriteFile(LanguageFile(), data, 0644); err != nil {
		return err
	}
	detectedLanguageCache.Store(int32(lang))
	return nil
}

// LoadLanguage loads the language preference from disk.
// Returns LangEN and false if no saved preference exists.
func LoadLanguage() (Language, bool) {
	data, err := os.ReadFile(LanguageFile())
	if err != nil {
		return LangEN, false
	}
	var cfg map[string]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		return LangEN, false
	}
	code, ok := cfg["language"]
	if !ok {
		return LangEN, false
	}
	for _, l := range AllLanguages() {
		if l.Code() == code {
			return l, true
		}
	}
	return LangEN, false
}

// DetectOrLoadLanguage detects the system language on first run,
// or loads the saved preference on subsequent runs.
func DetectOrLoadLanguage() Language {
	if cached := detectedLanguageCache.Load(); cached >= int32(LangEN) && cached <= int32(LangRU) {
		return Language(cached)
	}
	resolved := LangEN
	if lang, ok := LoadLanguage(); ok {
		resolved = lang
	} else {
		resolved = DetectLanguage()
	}
	detectedLanguageCache.CompareAndSwap(-1, int32(resolved))
	return Language(detectedLanguageCache.Load())
}
