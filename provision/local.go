package provision

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ErrNeedRoot означает, что запустили не под root. Пароль мы не спрашиваем и
// никуда не передаём: пусть человек сам решит, через что эскалировать.
var ErrNeedRoot = errors.New("нужны права root")

// Local выполняет скрипты на этой машине.
type Local struct{ Locale Lang }

func NewLocal(locale Lang) (*Local, error) {
	if err := checkLocalSupported(); err != nil {
		return nil, err
	}
	return &Local{Locale: locale}, nil
}

func (l *Local) Target() string { return tr(l.Locale, "this machine", "этой машине") }
func (l *Local) Close() error   { return nil }

func (l *Local) Run(ctx context.Context, script string, out func(string)) error {
	cmd := exec.CommandContext(ctx, "bash", "-s")
	// скрипт уходит в stdin, а не в argv, поэтому не светится в `ps`
	cmd.Stdin = strings.NewReader(script)

	// Отмена должна убивать не только bash, но и всё, что он породил. Иначе
	// apt и docker переживают Ctrl+C, продолжают держать унаследованные пайпы,
	// и чтение из них висит до конца установки. То есть отмена не отменяет.
	setupProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go streamLines(&wg, stdout, out)
	go streamLines(&wg, stderr, out)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		// если контекст отменён, возвращаем именно это, а не "signal: killed"
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	return nil
}

func streamLines(wg *sync.WaitGroup, r io.Reader, out func(string)) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // длинные строки docker build
	for sc.Scan() {
		out(sc.Text())
	}
}
