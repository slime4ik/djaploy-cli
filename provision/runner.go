// Package provision содержит шаги подготовки сервера: пакеты, fail2ban, Docker,
// отдельный пользователь для деплоя.
//
// Пакет публичный намеренно. Код, который едет на чужой сервер и работает там
// под root, должен быть открыт, иначе его нельзя проверить.
//
// Этот же пакет использует бэкенд djaploy, когда деплоит по SSH. Одна логика на
// оба пути, чтобы CLI и панель не разъехались.
package provision

import "context"

// Runner выполняет shell-скрипт с правами root и отдаёт вывод построчно.
//
// Реализации: Local (на этой машине), Remote (по SSH с ноутбука), DryRun
// (ничего не выполняет, только печатает).
type Runner interface {
	Run(ctx context.Context, script string, out func(string)) error

	// Target отвечает на вопрос "где выполняем": "этой машине", "root@1.2.3.4".
	Target() string

	Close() error
}

// sq заключает строку в одинарные кавычки для shell. Внутри всё воспринимается
// буквально, кроме самой кавычки: её закрываем, экранируем и открываем заново.
func sq(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\\', '\'', '\'')
			continue
		}
		out = append(out, s[i])
	}
	return string(append(out, '\''))
}
