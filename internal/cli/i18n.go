package cli

import (
	"os"
	"strings"

	"github.com/slime4ik/djaploy-cli/provision"
)

// lang задаёт язык вывода, определяется один раз при старте.
var lang = detectLang()

// t выбирает строку по текущему языку. Пары держим прямо в месте
// использования: перевод виден рядом с оригиналом и не теряется при
// рефакторинге, как это бывает с каталогом ключей.
func t(en, ru string) string {
	if lang == provision.LangRU {
		return ru
	}
	return en
}

// detectLang смотрит явный DJAPLOY_LANG, потом системную локаль.
//
// По умолчанию английский, потому что на минимальных образах VPS переменная
// LANG часто пуста или равна C, и угадывать там нечего. Русскоязычным лендинг
// показывает команду с DJAPLOY_LANG=ru.
func detectLang() provision.Lang {
	if v := os.Getenv("DJAPLOY_LANG"); v != "" {
		return normalizeLang(v)
	}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			return normalizeLang(v)
		}
	}
	return provision.LangEN
}

func normalizeLang(v string) provision.Lang {
	if strings.HasPrefix(strings.ToLower(v), "ru") {
		return provision.LangRU
	}
	return provision.LangEN
}
