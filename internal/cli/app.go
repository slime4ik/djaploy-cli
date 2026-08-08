// Package cli разбирает команды и печатает в терминал.
//
// Команды лежат в одном реестре, добавить новую стоит одну строку. Сетевые
// команды (login, ps, logs) встанут сюда же, когда появятся.
package cli

import (
	"fmt"
	"strings"
)

// Run это точка входа. Возвращает код выхода процесса.
func Run(args []string) int {
	// --lang вынимаем раньше всего: он влияет и на текст ошибок разбора
	args = applyLangFlag(args)

	if len(args) == 0 {
		fmt.Print(helpText())
		return 0
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "setup":
		return runSetup(rest)
	case "status":
		return runStatus(rest)
	case "update":
		return runUpdate(rest)
	case "uninstall":
		return runUninstall(rest)
	case "version", "--version", "-v":
		say("djaploy %s", Version)
		return 0
	case "help", "--help", "-h":
		fmt.Print(helpText())
		return 0
	default:
		fail(t("Unknown command %q.", "Неизвестная команда «%s»."), cmd)
		fail(t("Help: djaploy --help", "Справка: djaploy --help"))
		return 2
	}
}

// applyLangFlag выставляет язык из --lang и убирает флаг из аргументов, чтобы
// разбор команд его не видел. Работает для любой команды, не только setup.
func applyLangFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		name, inline, hasInline := strings.Cut(args[i], "=")
		if name != "--lang" {
			out = append(out, args[i])
			continue
		}
		switch {
		case hasInline:
			lang = normalizeLang(inline)
		case i+1 < len(args):
			i++
			lang = normalizeLang(args[i])
		}
	}
	return out
}
