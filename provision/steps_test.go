package provision

import (
	"strings"
	"testing"
)

// Обещания из README, проверяемые машиной. Если кто-то однажды допишет в шаг
// `ufw enable` или правку sshd_config, тест упадёт раньше, чем это уедет на
// чужой сервер.
func TestScriptsKeepSafetyPromises(t *testing.T) {
	forbidden := []struct {
		substr string
		why    string
	}{
		{"sshd_config", "sshd_config не трогаем: сломанный sshd означает потерянный сервер"},
		{"ufw enable", "ufw сами не включаем: можно отрезать текущую сессию"},
		{"ufw --force enable", "то же самое, но через --force"},
		{"iptables -D", "чужие правила iptables не удаляем"},
		{"iptables -F", "цепочки iptables не чистим"},
		{"rm -rf /", "защита от опечатки в пути"},
	}

	for _, lang := range []Lang{LangEN, LangRU} {
		for _, s := range Steps() {
			script := s.Script(Options{Locale: lang})
			for _, f := range forbidden {
				if strings.Contains(script, f.substr) {
					t.Errorf("шаг %s (%s) содержит %q: %s", s.Key, lang, f.substr, f.why)
				}
			}
		}
	}
}

func TestScriptsAreLocalised(t *testing.T) {
	for _, s := range Steps() {
		en := s.Script(Options{Locale: LangEN})
		ru := s.Script(Options{Locale: LangRU})

		if en == "" || ru == "" {
			t.Fatalf("шаг %s: пустой скрипт", s.Key)
		}
		if en == ru {
			t.Errorf("шаг %s: EN и RU скрипты совпадают, переводы не подставились", s.Key)
		}
		if s.Title(LangEN) == "" || s.Title(LangRU) == "" {
			t.Errorf("шаг %s: пустой заголовок", s.Key)
		}
		if s.Title(LangEN) == s.Title(LangRU) {
			t.Errorf("шаг %s: заголовок не переведён", s.Key)
		}
	}
}

// Пустой Lang должен давать английский, а не пустые строки.
func TestEmptyLocaleFallsBackToEnglish(t *testing.T) {
	var o Options
	if got := o.Lang(); got != LangEN {
		t.Fatalf("пустая локаль дала %q, ожидался %q", got, LangEN)
	}
	if s := stepBase.Title(""); s != stepBase.Title(LangEN) {
		t.Errorf("Title(\"\") = %q, ожидался английский", s)
	}
}

func TestNonRootUsesCustomUserAndKey(t *testing.T) {
	opts := Options{DeployUser: "builder", PubKey: "ssh-ed25519 AAAAKEY user@host"}
	script := stepNonRoot.Script(opts)

	if !strings.Contains(script, "'builder'") {
		t.Error("имя пользователя не подставилось в скрипт")
	}
	if !strings.Contains(script, "ssh-ed25519 AAAAKEY user@host") {
		t.Error("публичный ключ не подставился в скрипт")
	}
	// с явным ключом копировать ключи root не нужно
	if strings.Contains(script, "cat /root/.ssh/authorized_keys") {
		t.Error("при явном --pubkey скрипт всё равно копирует ключи root")
	}
}

// Без ключа шаг обязан подстраховаться ключами root, иначе получится
// пользователь, под которого никто не может зайти.
func TestNonRootFallsBackToRootKeys(t *testing.T) {
	script := stepNonRoot.Script(Options{})
	if !strings.Contains(script, "cat /root/.ssh/authorized_keys") {
		t.Error("без --pubkey скрипт не копирует ключи root, пользователь окажется недоступен")
	}
}

func TestApplyUpdatesState(t *testing.T) {
	st := &State{}
	opts := Options{DeployUser: "builder"}

	for _, s := range Steps() {
		s.apply(st, opts)
	}

	if !st.Fail2ban || !st.Docker || !st.RegistryMirrors {
		t.Errorf("состояние обновилось не полностью: %+v", st)
	}
	if st.DeployUser != "builder" {
		t.Errorf("DeployUser = %q, ожидался builder", st.DeployUser)
	}
}

func TestMirrorsGoIntoDaemonJSON(t *testing.T) {
	script := stepDocker.Script(Options{Mirrors: []string{"https://mirror.example"}})

	if !strings.Contains(script, "https://mirror.example") {
		t.Error("своё зеркало не попало в daemon.json")
	}
	if strings.Contains(script, DefaultMirrors[0]) {
		t.Error("зеркала по умолчанию не должны подмешиваться к своим")
	}
}

func TestStepByKey(t *testing.T) {
	for _, s := range Steps() {
		got, ok := StepByKey(s.Key)
		if !ok || got.Key != s.Key {
			t.Errorf("StepByKey(%q) не нашёл шаг", s.Key)
		}
	}
	if _, ok := StepByKey("nope"); ok {
		t.Error("StepByKey нашёл несуществующий шаг")
	}
}

func TestShellQuoting(t *testing.T) {
	cases := map[string]string{
		"deploy":       `'deploy'`,
		"it's":         `'it'\''s'`,
		"a b":          `'a b'`,
		"$(rm -rf /) ": `'$(rm -rf /) '`, // подстановка команд не должна сработать
	}
	for in, want := range cases {
		if got := sq(in); got != want {
			t.Errorf("sq(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

// Имя пользователя приходит снаружи и не должно уметь вырваться из кавычек.
func TestDeployUserCannotInjectShell(t *testing.T) {
	script := stepNonRoot.Script(Options{DeployUser: "x'; touch /tmp/pwned; '"})
	if strings.Contains(script, "; touch /tmp/pwned; ") &&
		!strings.Contains(script, `'\''`) {
		t.Error("имя пользователя вырвалось из кавычек, возможна инъекция")
	}
}
