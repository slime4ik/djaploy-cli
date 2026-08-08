package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/slime4ik/djaploy-cli/provision"
)

func TestParseKey(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
		want  key
	}{
		{"стрелка вверх", []byte{0x1b, '[', 'A'}, keyUp},
		{"стрелка вниз", []byte{0x1b, '[', 'B'}, keyDown},
		{"стрелка вправо игнорируется", []byte{0x1b, '[', 'C'}, keyNone},
		{"пробел", []byte{' '}, keySpace},
		{"enter", []byte{'\r'}, keyEnter},
		{"перевод строки", []byte{'\n'}, keyEnter},
		{"q", []byte{'q'}, keyQuit},
		{"esc", []byte{0x1b}, keyQuit},
		{"ctrl+c", []byte{0x03}, keyInterrupt},
		{"k как вверх", []byte{'k'}, keyUp},
		{"j как вниз", []byte{'j'}, keyDown},
		{"мусор", []byte{'z'}, keyNone},
	}
	for _, c := range cases {
		got, err := newKeyReader(bytes.NewReader(c.input)).next()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: получили %v, ожидалось %v", c.name, got, c.want)
		}
	}
}

// Настоящий баг, пойманный прогоном в pty: когда несколько нажатий приходят
// одним чтением (быстрый ввод, автоповтор стрелки, вставка), обрабатывалось
// только первое, остальные молча терялись.
func TestKeyReaderDoesNotDropKeysArrivingTogether(t *testing.T) {
	// вниз, вниз, пробел, enter — всё одним куском
	input := []byte("\x1b[B\x1b[B \r")

	kr := newKeyReader(bytes.NewReader(input))
	want := []key{keyDown, keyDown, keySpace, keyEnter}

	for i, w := range want {
		got, err := kr.next()
		if err != nil {
			t.Fatalf("нажатие %d: %v", i+1, err)
		}
		if got != w {
			t.Fatalf("нажатие %d: получили %v, ожидалось %v", i+1, got, w)
		}
	}
}

// Escape-последовательность может прийти по кускам: три отдельных чтения.
func TestKeyReaderHandlesSplitEscapeSequence(t *testing.T) {
	kr := newKeyReader(&chunkReader{chunks: [][]byte{{0x1b}, {'['}, {'A'}}})

	got, err := kr.next()
	if err != nil {
		t.Fatal(err)
	}
	if got != keyUp {
		t.Errorf("разорванная последовательность дала %v, ожидалась стрелка вверх", got)
	}
}

// Оборванный на середине Esc не должен подвесить чтение.
func TestKeyReaderSurvivesTruncatedEscape(t *testing.T) {
	got, err := newKeyReader(bytes.NewReader([]byte{0x1b, '['})).next()
	if err != nil {
		t.Fatal(err)
	}
	if got != keyQuit {
		t.Errorf("обрубок escape дал %v, ожидался выход", got)
	}
}

// chunkReader отдаёт данные заранее заданными порциями, изображая терминал,
// который присылает escape-последовательность не целиком.
type chunkReader struct {
	chunks [][]byte
	i      int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.i >= len(c.chunks) {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[c.i])
	c.i++
	return n, nil
}

func TestChosen(t *testing.T) {
	all := provision.Steps()
	picked := make([]bool, len(all))
	picked[len(all)-1] = true

	got := chosen(all, picked)
	if len(got) != 1 || got[0].Key != all[len(all)-1].Key {
		t.Errorf("chosen вернул %d шагов, ожидался последний", len(got))
	}

	// ничего не отмечено значит пустой список, а не nil-паника
	if got := chosen(all, make([]bool, len(all))); len(got) != 0 {
		t.Errorf("без отметок ожидался пустой список, получили %d", len(got))
	}
}

// renderMenu должен вернуть ровно столько строк, сколько напечатал: на этом
// держится перерисовка на месте. Ошибка здесь даёт уезжающий по экрану список.
func TestRenderMenuLineCount(t *testing.T) {
	all := provision.Steps()
	picked := make([]bool, len(all))

	out := captureStdout(t, func() {
		if n := renderMenu(all, picked, &provision.State{}, 0); n != wantLines(len(all)) {
			t.Errorf("renderMenu вернул %d строк, ожидалось %d", n, wantLines(len(all)))
		}
	})

	if got := strings.Count(out, "\r\n"); got != wantLines(len(all)) {
		t.Errorf("напечатано %d строк, а насчитано %d", got, wantLines(len(all)))
	}
}

// 3 строки шапки + по 2 на шаг + 2 строки подсказки
func wantLines(steps int) int { return 3 + steps*2 + 2 }

func TestRenderMenuMarksState(t *testing.T) {
	all := provision.Steps()
	picked := make([]bool, len(all))
	picked[0] = true

	out := captureStdout(t, func() {
		renderMenu(all, picked, &provision.State{Docker: true}, 1)
	})

	if !strings.Contains(out, "◉") {
		t.Error("отмеченный пункт не показан как выбранный")
	}
	if !strings.Contains(out, "◯") {
		t.Error("неотмеченный пункт не показан как невыбранный")
	}
	if !strings.Contains(out, "❯") {
		t.Error("курсор не нарисован")
	}
	// проверяем оба языка, чтобы тест не зависел от локали окружения
	if !strings.Contains(out, "already done") && !strings.Contains(out, "уже сделано") {
		t.Error("уже выполненный шаг не помечен")
	}
}

func TestValidUsernameThroughFlag(t *testing.T) {
	var opts provision.Options
	steps := []provision.Step{mustStep(t, "nonroot")}

	// явный --user проходит проверку и подставляется
	if err := resolveDeployUser(&opts, steps, setupFlags{user: "builder"}); err != nil {
		t.Fatalf("годное имя отвергнуто: %v", err)
	}
	if opts.DeployUser != "builder" {
		t.Errorf("DeployUser = %q", opts.DeployUser)
	}

	// негодное отвергается до запуска, а не в середине прогона
	if err := resolveDeployUser(&provision.Options{}, steps, setupFlags{user: "Root Boy!"}); err == nil {
		t.Error("негодное имя принято")
	}

	// --yes без --user берёт deploy молча
	var auto provision.Options
	if err := resolveDeployUser(&auto, steps, setupFlags{yes: true}); err != nil {
		t.Fatal(err)
	}
	if auto.DeployUser != "deploy" {
		t.Errorf("под --yes ожидался deploy, получили %q", auto.DeployUser)
	}

	// без шага nonroot имя вообще не трогаем
	var untouched provision.Options
	if err := resolveDeployUser(&untouched, []provision.Step{mustStep(t, "docker")}, setupFlags{}); err != nil {
		t.Fatal(err)
	}
	if untouched.DeployUser != "" {
		t.Errorf("без шага nonroot имя не должно выставляться, получили %q", untouched.DeployUser)
	}
}

func mustStep(t *testing.T, key string) provision.Step {
	t.Helper()
	s, ok := provision.StepByKey(key)
	if !ok {
		t.Fatalf("нет шага %q", key)
	}
	return s
}
