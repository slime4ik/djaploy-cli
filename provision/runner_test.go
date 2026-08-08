package provision

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Local строим напрямую, минуя NewLocal: проверяем механику выполнения, а не
// проверку прав. Скрипты здесь безобидные, root не нужен.
func TestLocalRunnerStreamsOutput(t *testing.T) {
	r := &Local{}
	var lines []string

	err := r.Run(context.Background(),
		"echo первая\necho вторая >&2\necho третья",
		func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	got := strings.Join(lines, "|")
	for _, want := range []string{"первая", "вторая", "третья"} {
		if !strings.Contains(got, want) {
			t.Errorf("строка %q не попала в вывод: %q", want, got)
		}
	}
}

func TestLocalRunnerPropagatesExitCode(t *testing.T) {
	r := &Local{}
	err := r.Run(context.Background(), "set -e\nexit 42", func(string) {})
	if err == nil {
		t.Fatal("ненулевой код возврата не превратился в ошибку")
	}
}

func TestLocalRunnerRespectsContext(t *testing.T) {
	r := &Local{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := r.Run(ctx, "sleep 10", func(string) {}); err == nil {
		t.Fatal("долгий скрипт не был прерван по контексту")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("прерывание заняло %v, контекст не сработал", elapsed)
	}
}

func TestDryRunExecutesNothing(t *testing.T) {
	marker := "/tmp/djaploy-dryrun-must-not-exist"
	r := &DryRun{}

	var printed []string
	err := r.Run(context.Background(), "touch "+marker, func(l string) {
		printed = append(printed, l)
	})
	if err != nil {
		t.Fatalf("DryRun вернул ошибку: %v", err)
	}
	if len(printed) != 1 || printed[0] != "touch "+marker {
		t.Errorf("DryRun должен печатать скрипт дословно, получили %q", printed)
	}

	// проверяем через настоящий bash, что файла нет
	var out []string
	_ = (&Local{}).Run(context.Background(),
		"test -e "+marker+" && echo EXISTS || echo MISSING",
		func(l string) { out = append(out, l) })
	if len(out) == 0 || out[0] != "MISSING" {
		t.Errorf("DryRun что-то выполнил: %v", out)
	}
}

// Состояние должно пережить запись и чтение через один и тот же Runner.
func TestStateRoundTripThroughRunner(t *testing.T) {
	fake := &memRunner{}
	st := &State{Fail2ban: true, Docker: true, DeployUser: "builder"}

	if err := st.Save(context.Background(), fake, "1.2.3"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if st.UpdatedAt == "" || st.CLIVersion != "1.2.3" {
		t.Error("Save не проставил UpdatedAt/CLIVersion")
	}

	back := ReadState(context.Background(), fake)
	if !back.Fail2ban || !back.Docker || back.DeployUser != "builder" {
		t.Errorf("состояние не восстановилось: %+v", back)
	}
	if back.CLIVersion != "1.2.3" {
		t.Errorf("версия не сохранилась: %q", back.CLIVersion)
	}
}

// Битый state.json не должен ронять инструмент, начинаем с чистого состояния.
func TestReadStateSurvivesGarbage(t *testing.T) {
	fake := &memRunner{stored: "{ это не json"}
	if got := ReadState(context.Background(), fake); got == nil || got.Docker {
		t.Errorf("битый файл дал %+v, ожидалось пустое состояние", got)
	}
}

// memRunner изображает файловую систему: запоминает то, что записали
// heredoc'ом, и отдаёт обратно на `cat`. Настоящий bash тут не нужен.
type memRunner struct{ stored string }

func (m *memRunner) Target() string { return "memory" }
func (m *memRunner) Close() error   { return nil }

func (m *memRunner) Run(_ context.Context, script string, out func(string)) error {
	if _, body, ok := strings.Cut(script, "<<'DJAPLOY_STATE'\n"); ok {
		m.stored, _, _ = strings.Cut(body, "\nDJAPLOY_STATE")
		return nil
	}
	if strings.HasPrefix(script, "cat ") {
		for _, l := range strings.Split(m.stored, "\n") {
			out(l)
		}
	}
	return nil
}
