package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Цвета включаем только для настоящего терминала. В пайпе и в CI они
// превратились бы в мусор посреди логов.
var colored = term.IsTerminal(int(os.Stdout.Fd()))

func paint(code, s string) string {
	if !colored {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func bold(s string) string   { return paint("1", s) }
func dim(s string) string    { return paint("2", s) }
func green(s string) string  { return paint("32", s) }
func red(s string) string    { return paint("31", s) }
func yellow(s string) string { return paint("33", s) }

func say(format string, a ...any)  { fmt.Fprintf(os.Stdout, format+"\n", a...) }
func fail(format string, a ...any) { fmt.Fprintf(os.Stderr, format+"\n", a...) }

var stdin = bufio.NewReader(os.Stdin)

func ask(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	line, err := stdin.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// confirm задаёт вопрос да/нет. Пустой ответ равен def, чтобы Enter делал ожидаемое.
func confirm(prompt string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	answer, err := ask(prompt + " " + hint + " ")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "":
		return def, nil
	case "y", "yes", "д", "да":
		return true, nil
	default:
		return false, nil
	}
}

// askPassword читает пароль без эха.
func askPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("пароль нужен, но ввод не терминал")
	}
	raw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
