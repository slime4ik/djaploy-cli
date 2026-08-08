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

// detectLang смотрит только явный DJAPLOY_LANG.
//
// Системную локаль намеренно не читаем. Проект открытый и международный,
// поэтому язык по умолчанию английский и одинаковый у всех: инструмент,
// который на одной машине говорит по-русски, а на другой по-английски, ведёт
// себя непредсказуемо. Русский включается явно, флагом или переменной.
func detectLang() provision.Lang {
	if v := os.Getenv("DJAPLOY_LANG"); v != "" {
		return normalizeLang(v)
	}
	return provision.LangEN
}

func normalizeLang(v string) provision.Lang {
	if strings.HasPrefix(strings.ToLower(v), "ru") {
		return provision.LangRU
	}
	return provision.LangEN
}
