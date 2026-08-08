package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/slime4ik/djaploy-cli/provision"
	"golang.org/x/term"
)

// selectSteps даёт выбрать шаги.
//
// В настоящем терминале это меню со стрелками. Если ввод не терминал (пайп,
// CI, `ssh host djaploy setup`), падаем на ввод номеров: raw-режим там
// невозможен, а работать инструмент обязан.
func selectSteps(state *provision.State) ([]provision.Step, error) {
	all := provision.Steps()
	picked := make([]bool, len(all))
	for i, s := range all {
		picked[i] = s.Default
	}

	if term.IsTerminal(int(os.Stdin.Fd())) && colored {
		return arrowMenu(all, picked, state)
	}

	// Откат на цифры не должен выглядеть как "так и задумано": человек, который
	// ждал стрелок, иначе решит, что инструмент просто древний.
	if colored {
		say("%s", dim(t(
			"(input is not a terminal, falling back to numbers)",
			"(ввод не терминал, поэтому выбор цифрами)")))
	}
	return numberedMenu(all, picked, state)
}

// Управляющие последовательности. Их немного, и держать их именованными
// понятнее, чем встречать \x1b[?25l посреди кода.
const (
	hideCursor  = "\x1b[?25l"
	showCursor  = "\x1b[?25h"
	clearToEnd  = "\x1b[0J"
	cursorUpFmt = "\x1b[%dA"
)

// arrowMenu рисует список и обновляет его на месте, перерисовывая только свои
// строки. Стрелки, пробел, enter.
func arrowMenu(all []provision.Step, picked []bool, state *provision.State) ([]provision.Step, error) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		// терминал необычный: не падаем, но и не молчим, иначе непонятно,
		// почему вместо стрелок цифры
		say("%s", dim(fmt.Sprintf(t(
			"(this terminal does not support arrow keys: %v)",
			"(этот терминал не поддерживает стрелки: %v)"), err)))
		return numberedMenu(all, picked, state)
	}
	// Восстановить терминал надо при любом выходе, включая панику: иначе
	// человек останется в шелле без эха и переносов строк.
	defer func() {
		term.Restore(fd, old)
		fmt.Print(showCursor)
	}()
	fmt.Print(hideCursor)

	keys := newKeyReader(os.Stdin)
	cursor := 0
	lines := 0

	for {
		if lines > 0 {
			fmt.Printf(cursorUpFmt, lines)
		}
		fmt.Print("\r" + clearToEnd)
		lines = renderMenu(all, picked, state, cursor)

		key, err := keys.next()
		if err != nil {
			return nil, err
		}

		switch key {
		case keyUp:
			cursor = (cursor - 1 + len(all)) % len(all)
		case keyDown:
			cursor = (cursor + 1) % len(all)
		case keySpace:
			picked[cursor] = !picked[cursor]
		case keyEnter:
			return chosen(all, picked), nil
		case keyQuit:
			return nil, nil
		case keyInterrupt:
			return nil, errInterrupted
		}
	}
}

// renderMenu печатает меню и возвращает, сколько строк заняло, чтобы в
// следующий раз перерисовать ровно их.
func renderMenu(all []provision.Step, picked []bool, state *provision.State, cursor int) int {
	var b strings.Builder
	n := 0
	line := func(s string) {
		b.WriteString(s + "\r\n")
		n++
	}

	line("")
	line("  " + bold(t("What should be done to the server?", "Что сделать с сервером?")))
	line("")

	for i, s := range all {
		mark := "◯"
		if picked[i] {
			mark = green("◉")
		}
		pointer := "  "
		title := s.Title(lang)
		if i == cursor {
			pointer = green("❯") + " "
			title = bold(title)
		}
		done := ""
		if alreadyDone(state, s.Key) {
			done = dim(t("  already done", "  уже сделано"))
		}
		line(pointer + mark + " " + title + done)
		line("      " + dim(s.Help(lang)))
	}

	line("")
	line("  " + dim(t(
		"↑↓ move · space select · enter run · q quit",
		"↑↓ выбор · пробел отметить · enter запустить · q выход")))

	fmt.Print(b.String())
	return n
}

func chosen(all []provision.Step, picked []bool) []provision.Step {
	out := []provision.Step{}
	for i, s := range all {
		if picked[i] {
			out = append(out, s)
		}
	}
	return out
}

type key int

const (
	keyNone key = iota
	keyUp
	keyDown
	keySpace
	keyEnter
	keyQuit
	keyInterrupt
)

// escapeWait — сколько ждём продолжение escape-последовательности.
//
// Одиночный Esc и начало стрелки выглядят одинаково: оба начинаются с 0x1b.
// Без ожидания стрелка, пришедшая по частям (обычное дело на медленном SSH),
// прочиталась бы как Esc, то есть как выход из меню.
const escapeWait = 50 * time.Millisecond

// deadlineReader — то, что умеет ограничивать время чтения. Терминал умеет,
// bytes.Reader в тестах нет, поэтому проверяем приведением.
type deadlineReader interface{ SetReadDeadline(time.Time) error }

// keyReader читает нажатия по одному байту.
//
// Побайтово намеренно: читать с запасом нельзя, иначе меню проглотит символы,
// которые человек набрал для следующего вопроса.
type keyReader struct {
	r   io.Reader
	one []byte
}

func newKeyReader(r io.Reader) *keyReader {
	return &keyReader{r: r, one: make([]byte, 1)}
}

func (k *keyReader) next() (key, error) {
	b, err := k.readByte(false)
	if err != nil {
		return keyNone, err
	}
	if b != 0x1b {
		return simpleKey(b), nil
	}

	// Дальше либо ничего (одиночный Esc), либо '[' и код стрелки.
	if b, err := k.readByte(true); err != nil || b != '[' {
		return keyQuit, nil
	}
	b, err = k.readByte(true)
	if err != nil {
		return keyQuit, nil
	}
	switch b {
	case 'A':
		return keyUp, nil
	case 'B':
		return keyDown, nil
	}
	return keyNone, nil // стрелки вбок и прочее нам не нужны
}

// readByte читает один байт. Внутри escape-последовательности ждёт недолго:
// если продолжения нет, значит это был просто Esc.
func (k *keyReader) readByte(inEscape bool) (byte, error) {
	if d, ok := k.r.(deadlineReader); ok && inEscape {
		_ = d.SetReadDeadline(time.Now().Add(escapeWait))
		defer d.SetReadDeadline(time.Time{})
	}
	for {
		n, err := k.r.Read(k.one)
		if n > 0 {
			return k.one[0], nil
		}
		if err != nil {
			return 0, err
		}
	}
}

func simpleKey(b byte) key {
	switch b {
	case 0x03: // Ctrl+C. В raw-режиме это байт, а не сигнал, ловим руками.
		return keyInterrupt
	case ' ':
		return keySpace
	case '\r', '\n':
		return keyEnter
	case 'q', 'Q':
		return keyQuit
	case 'k', 'K': // привычка из vim
		return keyUp
	case 'j', 'J':
		return keyDown
	}
	return keyNone
}

// numberedMenu это запасной вариант для не-терминала: пайп, CI, скрипт.
func numberedMenu(all []provision.Step, picked []bool, state *provision.State) ([]provision.Step, error) {
	for {
		say("")
		say("%s", bold(t("What should be done to the server?", "Что сделать с сервером?")))
		say("")
		for i, s := range all {
			box := "[ ]"
			if picked[i] {
				box = "[x]"
			}
			done := ""
			if alreadyDone(state, s.Key) {
				done = green(t(" already done", " уже сделано"))
			}
			say("  %d %s %s%s", i+1, box, s.Title(lang), done)
			say("      %s", dim(s.Help(lang)))
		}
		say("")
		say("%s", dim(t(
			"numbers toggle · Enter runs · q quits",
			"номера переключают · Enter запускает · q выходит")))

		answer, err := ask("> ")
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(answer) {
		case "":
			return chosen(all, picked), nil
		case "q", "quit", "exit":
			return nil, nil
		}

		for _, tok := range strings.Fields(answer) {
			n := 0
			if _, err := fmt.Sscanf(tok, "%d", &n); err != nil || n < 1 || n > len(all) {
				say("%s %s", yellow("!"), fmt.Sprintf(t("did not understand %q", "не понял «%s»"), tok))
				continue
			}
			picked[n-1] = !picked[n-1]
		}
	}
}
