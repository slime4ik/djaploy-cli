package cli

import (
	"io"
	"os"
	"testing"
)

// captureStdout перехватывает то, что функция печатает в stdout. Нужен, чтобы
// проверять отрисовку меню, не имея настоящего терминала.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = saved }()

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	w.Close()
	return <-done
}
