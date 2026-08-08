package provision

import (
	"context"
	"time"
)

// Options собирает то, что настраивается снаружи. Остальное зафиксировано
// намеренно: чем меньше ручек, тем меньше способов сломать себе сервер.
type Options struct {
	// DeployUser задаёт имя пользователя для шага nonroot, по умолчанию "deploy".
	DeployUser string

	// PubKey это публичный SSH-ключ для deploy-пользователя. Если пусто,
	// скопируем authorized_keys текущего root, чтобы доступ не потерялся.
	PubKey string

	// Mirrors задаёт зеркала Docker-реестра. Если пусто, берём DefaultMirrors.
	Mirrors []string

	// Locale задаёт язык сообщений. Пусто означает английский.
	Locale Lang
}

// DefaultMirrors выручают, когда Docker Hub недоступен напрямую.
var DefaultMirrors = []string{
	"https://mirror.gcr.io",
	"https://dockerhub.timeweb.cloud",
}

// Step это один пункт меню: скрипт плюс то, что о нём нужно знать человеку.
type Step struct {
	Key     string        // машинное имя для --only
	Default bool          // отмечен ли по умолчанию
	Timeout time.Duration // сколько ждём до принудительной остановки

	title text // строка в меню
	help  text // что делает, одной фразой
	hint  text // подсказка, если шаг упал

	script func(Options) string
	apply  func(*State, Options)
}

func (s Step) Title(l Lang) string { return s.title.get(l) }
func (s Step) Help(l Lang) string  { return s.help.get(l) }
func (s Step) Hint(l Lang) string  { return s.hint.get(l) }

// Script возвращает текст, который уйдёт в bash. Вынесен в отдельный метод,
// чтобы --dry-run мог его показать, ничего не выполняя.
func (s Step) Script(opts Options) string { return s.script(opts) }

// Run выполняет шаг и при успехе обновляет состояние.
func (s Step) Run(ctx context.Context, r Runner, opts Options, st *State, out func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	if err := r.Run(ctx, s.script(opts), out); err != nil {
		return err
	}
	if s.apply != nil && st != nil {
		s.apply(st, opts)
	}
	return nil
}

// Steps возвращает все шаги в порядке выполнения. Порядок важен: docker ждёт,
// что база уже стоит, а nonroot ожидает существующую группу docker.
func Steps() []Step {
	return []Step{stepBase, stepDocker, stepNonRoot}
}

// StepByKey ищет шаг по машинному имени, для --only.
func StepByKey(key string) (Step, bool) {
	for _, s := range Steps() {
		if s.Key == key {
			return s, true
		}
	}
	return Step{}, false
}

func deployUser(opts Options) string {
	if opts.DeployUser == "" {
		return "deploy"
	}
	return opts.DeployUser
}

func mirrors(opts Options) []string {
	if len(opts.Mirrors) == 0 {
		return DefaultMirrors
	}
	return opts.Mirrors
}
