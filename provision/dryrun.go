package provision

import "context"

// DryRun ничего не выполняет, а печатает скрипт, который выполнился бы.
//
// На этом держится аудит: `djaploy setup --dry-run` показывает тот же текст,
// который иначе ушёл бы в bash. Не пересказ и не документацию, а сам код.
type DryRun struct {
	Locale  Lang
	Wrapped Runner // чей Target() показываем; может быть nil
}

func (d *DryRun) Target() string {
	if d.Wrapped != nil {
		return d.Wrapped.Target()
	}
	return tr(d.Locale, "this machine", "этой машине")
}

func (d *DryRun) Close() error {
	if d.Wrapped != nil {
		return d.Wrapped.Close()
	}
	return nil
}

func (d *DryRun) Run(_ context.Context, script string, out func(string)) error {
	for _, line := range splitLines(script) {
		out(line)
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
