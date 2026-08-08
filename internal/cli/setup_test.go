package cli

import (
	"testing"

	"github.com/slime4ik/djaploy-cli/provision"
)

func TestParseAddr(t *testing.T) {
	cases := []struct {
		in               string
		user, host, port string
	}{
		{"1.2.3.4", "root", "1.2.3.4", ""},
		{"deploy@1.2.3.4", "deploy", "1.2.3.4", ""},
		{"deploy@example.com:2222", "deploy", "example.com", "2222"},
		{"example.com:2222", "root", "example.com", "2222"},
	}
	for _, c := range cases {
		user, host, port := parseAddr(c.in)
		if user != c.user || host != c.host || port != c.port {
			t.Errorf("parseAddr(%q) = %q/%q/%q, ожидалось %q/%q/%q",
				c.in, user, host, port, c.user, c.host, c.port)
		}
	}
}

func TestParseSetupFlags(t *testing.T) {
	var f setupFlags
	args := []string{"--dry-run", "--only=base,docker", "--user", "builder", "-y"}
	if err := parseSetupFlags(args, &f); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !f.dryRun || !f.yes {
		t.Error("булевы флаги не выставились")
	}
	if f.only != "base,docker" {
		t.Errorf("only = %q", f.only)
	}
	if f.user != "builder" {
		t.Errorf("user = %q", f.user)
	}
}

func TestParseSetupFlagsRejectsGarbage(t *testing.T) {
	var f setupFlags
	if err := parseSetupFlags([]string{"--nope"}, &f); err == nil {
		t.Error("неизвестный флаг принят молча")
	}
	if err := parseSetupFlags([]string{"--only"}, &f); err == nil {
		t.Error("флаг без значения принят молча")
	}
}

// Опечатка в --only должна ловиться до подключения к серверу.
func TestStepsFromFlags(t *testing.T) {
	steps, err := stepsFromFlags(setupFlags{only: "base,nonroot"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(steps) != 2 || steps[0].Key != "base" || steps[1].Key != "nonroot" {
		t.Errorf("получили %d шагов, порядок нарушен", len(steps))
	}

	if _, err := stepsFromFlags(setupFlags{only: "bogus"}); err == nil {
		t.Error("несуществующий шаг принят")
	}

	// без флагов возвращается nil, значит нужно меню
	if steps, err := stepsFromFlags(setupFlags{}); err != nil || steps != nil {
		t.Error("без флагов ожидался nil (показать меню)")
	}

	// --yes даёт только шаги по умолчанию
	steps, err = stepsFromFlags(setupFlags{yes: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if !s.Default {
			t.Errorf("шаг %s не помечен Default, но попал в --yes", s.Key)
		}
	}
}

func TestNormalizeLang(t *testing.T) {
	cases := map[string]provision.Lang{
		"ru":          provision.LangRU,
		"ru_RU.UTF-8": provision.LangRU,
		"RU":          provision.LangRU,
		"en_US.UTF-8": provision.LangEN,
		"C":           provision.LangEN,
		"":            provision.LangEN,
		"de_DE.UTF-8": provision.LangEN,
	}
	for in, want := range cases {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

func TestApplyLangFlagStripsItFromArgs(t *testing.T) {
	orig := lang
	defer func() { lang = orig }()

	rest := applyLangFlag([]string{"setup", "--lang", "ru", "--dry-run"})
	if lang != provision.LangRU {
		t.Errorf("язык не применился: %q", lang)
	}
	if len(rest) != 2 || rest[0] != "setup" || rest[1] != "--dry-run" {
		t.Errorf("--lang не вырезан из аргументов: %v", rest)
	}

	rest = applyLangFlag([]string{"status", "--lang=en"})
	if lang != provision.LangEN {
		t.Errorf("--lang=en не применился: %q", lang)
	}
	if len(rest) != 1 || rest[0] != "status" {
		t.Errorf("--lang=en не вырезан: %v", rest)
	}
}
