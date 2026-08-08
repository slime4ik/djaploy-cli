package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/slime4ik/djaploy-cli/provision"
)

type setupFlags struct {
	dryRun bool
	yes    bool
	only   string
	remote string
	pubKey string
	user   string
}

func runSetup(args []string) int {
	var f setupFlags
	if err := parseSetupFlags(args, &f); err != nil {
		fail("%v", err)
		return 2
	}

	opts := provision.Options{DeployUser: f.user, PubKey: f.pubKey, Locale: lang}

	// Сначала то, что не требует доступа к серверу: опечатка в --only должна
	// ловиться сразу, а не после «нужны права root».
	steps, err := stepsFromFlags(f)
	if err != nil {
		fail("%v", err)
		return 2
	}

	// Ctrl+C прерывает текущий шаг, а не роняет процесс на середине apt
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner, err := makeRunner(ctx, f)
	if err != nil {
		fail("%v", err)
		return 1
	}
	defer runner.Close()

	state := provision.ReadState(ctx, runner)

	if steps == nil {
		if steps, err = menu(state); err != nil {
			fail("%v", err)
			return 1
		}
	}
	if len(steps) == 0 {
		say("%s", t("Nothing selected.", "Ничего не выбрано."))
		return 0
	}

	if f.dryRun {
		printDryRun(steps, opts, runner.Target())
		return 0
	}

	if !f.yes {
		say("")
		say(t("Will run on %s:", "Выполню на %s:"), bold(runner.Target()))
		for _, s := range steps {
			say("  • %s", s.Title(lang))
		}
		say("")
		ok, err := confirm(t("Go ahead?", "Поехали?"), true)
		if err != nil || !ok {
			say("%s", t("Cancelled.", "Отменено."))
			return 0
		}
	}

	return execute(ctx, runner, steps, opts, state)
}

func execute(ctx context.Context, r provision.Runner, steps []provision.Step, opts provision.Options, state *provision.State) int {
	for i, s := range steps {
		say("")
		say("%s %s", dim(fmt.Sprintf("[%d/%d]", i+1, len(steps))), bold(s.Title(lang)))

		err := s.Run(ctx, r, opts, state, func(line string) {
			say("  %s", dim(line))
		})
		if err != nil {
			say("")
			// Состояние уже прошедших шагов сохраняем в любом случае: и при
			// ошибке, и при Ctrl+C. Иначе повторный запуск начнёт всё заново.
			// Контекст здесь свежий, старый уже отменён и запись бы не прошла.
			_ = state.Save(context.Background(), r, Version)

			if errors.Is(err, context.Canceled) {
				fail("%s %s", yellow("!"), t(
					"Interrupted. What succeeded is saved, a rerun continues from there.",
					"Прервано. Что успели, то записано, повторный запуск продолжит."))
				return 130
			}
			fail("%s %s", red("✗"), fmt.Sprintf(t("Step %q failed: %v", "Шаг «%s» не прошёл: %v"), s.Title(lang), err))
			if h := s.Hint(lang); h != "" {
				fail("  %s", h)
			}
			return 1
		}
		say("  %s %s", green("✓"), t("done", "готово"))
	}

	if err := state.Save(ctx, r, Version); err != nil {
		fail("%s %s", yellow("!"), fmt.Sprintf(t(
			"Steps completed, but the state file could not be written: %v",
			"Шаги прошли, но состояние не записалось: %v"), err))
	}

	say("")
	say("%s %s", green("✓"), t("Server is ready.", "Сервер готов."))
	say("%s", dim(t("What's configured: djaploy status", "Что настроено: djaploy status")))
	return 0
}

// makeRunner выбирает, где выполнять: локально, по SSH или вхолостую.
func makeRunner(ctx context.Context, f setupFlags) (provision.Runner, error) {
	// для --dry-run не нужен ни root, ни подключение, мы ничего не запускаем
	if f.dryRun {
		return &provision.DryRun{Locale: lang}, nil
	}

	if f.remote == "" {
		r, err := provision.NewLocal(lang)
		if errors.Is(err, provision.ErrNeedRoot) {
			return nil, errors.New(t(
				"root required, run: sudo djaploy setup\n"+
					"You can see what would run without root: djaploy setup --dry-run",
				"нужны права root, запусти: sudo djaploy setup\n"+
					"Посмотреть, что будет выполнено, можно и без root: djaploy setup --dry-run"))
		}
		return r, err
	}

	user, host, port := parseAddr(f.remote)
	return provision.Dial(ctx, provision.RemoteConfig{
		User: user,
		Host: host,
		Port: port,

		HostKeyPrompt: func(h, fp string) (bool, error) {
			say("")
			say(t("Server %s has not been seen before.", "Сервер %s раньше не встречался."), bold(h))
			say(t("Key fingerprint: %s", "Отпечаток ключа: %s"), bold(fp))
			say("%s", dim(t(
				"Compare it with what your provider's panel shows. That is how key substitution is caught.",
				"Сверь его с тем, что показывает панель провайдера. Так проверяют подмену.")))
			return confirm(t("Trust this server?", "Доверяем этому серверу?"), false)
		},
		SudoPassword: func() (string, error) {
			say("%s", dim(t(
				"Not logged in as root, system steps need sudo.",
				"Вход не под root, для системных шагов нужен sudo.")))
			return askPassword(fmt.Sprintf(t("sudo password for %s: ", "Пароль sudo для %s: "), user))
		},
	})
}

// stepsFromFlags собирает шаги из одних флагов, без обращения к серверу.
// Возвращает nil, если флаги выбор не определяют и нужно показать меню.
func stepsFromFlags(f setupFlags) ([]provision.Step, error) {
	if f.only != "" {
		out := []provision.Step{}
		for _, key := range strings.Split(f.only, ",") {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			s, ok := provision.StepByKey(key)
			if !ok {
				return nil, fmt.Errorf(t("no step %q. Available: %s", "нет шага «%s». Доступны: %s"), key, allKeys())
			}
			out = append(out, s)
		}
		return out, nil
	}

	if f.yes {
		out := []provision.Step{}
		for _, s := range provision.Steps() {
			if s.Default {
				out = append(out, s)
			}
		}
		return out, nil
	}

	return nil, nil
}

// menu даёт выбрать шаги номерами. Простой ввод вместо TUI работает в любом
// терминале, включая кривую SSH-сессию, и не тянет зависимостей.
func menu(state *provision.State) ([]provision.Step, error) {
	all := provision.Steps()
	picked := make([]bool, len(all))
	for i, s := range all {
		picked[i] = s.Default
	}

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
			out := []provision.Step{}
			for i, s := range all {
				if picked[i] {
					out = append(out, s)
				}
			}
			return out, nil
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

func alreadyDone(s *provision.State, key string) bool {
	switch key {
	case "base":
		return s.Fail2ban
	case "docker":
		return s.Docker
	case "nonroot":
		return s.DeployUser != ""
	}
	return false
}

func printDryRun(steps []provision.Step, opts provision.Options, target string) {
	say("%s", bold(fmt.Sprintf(t(
		"Running nothing. This is what would run on %s.",
		"Ничего не выполняю. Вот что выполнилось бы на %s."), target)))
	for _, s := range steps {
		say("")
		say("%s", bold("# ─── "+s.Title(lang)+" ("+s.Key+")"))
		say("%s", s.Script(opts))
	}
}

func allKeys() string {
	var keys []string
	for _, s := range provision.Steps() {
		keys = append(keys, s.Key)
	}
	return strings.Join(keys, ", ")
}

// parseAddr разбирает user@host:port. Без user подставляет root, без порта 22.
func parseAddr(addr string) (user, host, port string) {
	user, host = "root", addr
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		user, host = addr[:i], addr[i+1:]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host, port = host[:i], host[i+1:]
	}
	return user, host, port
}

func parseSetupFlags(args []string, f *setupFlags) error {
	// значение может идти как «--only base», так и «--only=base»
	value := func(i *int, name, inline string) (string, error) {
		if inline != "" {
			return inline, nil
		}
		if *i+1 >= len(args) {
			return "", fmt.Errorf(t("flag %s has no value", "у флага %s нет значения"), name)
		}
		*i++
		return args[*i], nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inline, _ := strings.Cut(arg, "=")

		var err error
		switch name {
		case "--dry-run":
			f.dryRun = true
		case "--yes", "-y":
			f.yes = true
		case "--only":
			f.only, err = value(&i, name, inline)
		case "--remote":
			f.remote, err = value(&i, name, inline)
		case "--pubkey":
			f.pubKey, err = value(&i, name, inline)
		case "--user":
			f.user, err = value(&i, name, inline)
		default:
			return fmt.Errorf(t("unknown flag %q. Help: djaploy --help",
				"неизвестный флаг «%s». Справка: djaploy --help"), arg)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
