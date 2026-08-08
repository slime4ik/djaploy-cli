package provision

import (
	"encoding/json"
	"strings"
	"time"
)

// Правила, которых держатся все шаги:
//
//   - sshd_config не трогаем никогда. Сломанный sshd означает сервер, до
//     которого владелец больше не достучится.
//   - `ufw enable` сами не делаем. Если ufw уже включён, только открываем
//     нужные порты. Включить фаервол за человека, который не просил, значит
//     рискнуть отрезать ему текущую сессию.
//   - чужие правила iptables не удаляем.
//   - каждый шаг идемпотентен: второй запуск ничего не ломает.

var stepBase = Step{
	Key:     "base",
	Default: true,
	Timeout: 8 * time.Minute,

	title: text{
		en: "Update packages and install fail2ban",
		ru: "Обновить пакеты и поставить fail2ban",
	},
	help: text{
		en: "apt update/install, basic tools, SSH brute-force protection",
		ru: "apt update/install, базовые утилиты, бан перебора SSH-паролей",
	},
	hint: text{
		en: "Needs Ubuntu/Debian with systemd and internet access. The exact error is in the log above.",
		ru: "Нужен Ubuntu/Debian с systemd и доступом в интернет. Точная строка ошибки есть в логе выше.",
	},

	apply: func(s *State, _ Options) { s.Fail2ban = true },

	script: func(o Options) string {
		l := o.Lang()
		return `set -e
export DEBIAN_FRONTEND=noninteractive

# DPkg::Lock::Timeout=300 makes apt WAIT for the lock for up to 5 minutes.
# On a fresh server a background apt-daily/unattended-upgrades run holds it,
# and without waiting we would fail with "Could not get lock" for no reason.
#
# We do not abort if a THIRD-PARTY repository is broken (a stale PPA, say):
# the packages we need live in the distribution's own repository.
apt-get -o DPkg::Lock::Timeout=300 update -y || echo ` + sq(tr(l,
			"(apt-get update partially failed, likely a third-party repo; continuing)",
			"(apt-get update прошёл частично, вероятно из-за стороннего репозитория; продолжаю)")) + `
apt-get -o DPkg::Lock::Timeout=300 install -y -o Dpkg::Options::=--force-confnew \
  fail2ban curl ca-certificates git

cat > /etc/fail2ban/jail.local <<'INI'
[sshd]
enabled = true
maxretry = 5
bantime = 1h
INI

systemctl enable --now fail2ban
systemctl restart fail2ban

# Caddy and Let's Encrypt need HTTP/HTTPS. We open them ONLY if ufw is already
# enabled; we never enable it ourselves. A cloud provider's firewall is separate.
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
  ufw allow 80/tcp >/dev/null 2>&1 || true
  ufw allow 443/tcp >/dev/null 2>&1 || true
  echo ` + sq(tr(l, "ufw is active, opened 80/443", "ufw активен, открыл 80/443")) + `
fi

echo ` + sq(tr(l,
			"Done: packages updated, fail2ban running.",
			"Готово: пакеты обновлены, fail2ban работает.")) + `
`
	},
}

var stepDocker = Step{
	Key:     "docker",
	Default: true,
	Timeout: 8 * time.Minute,

	title: text{
		en: "Docker and registry mirrors",
		ru: "Docker и зеркала реестра",
	},
	help: text{
		en: "installs Docker via get.docker.com and configures registry mirrors",
		ru: "ставит Docker с get.docker.com и прописывает зеркала на случай блокировок",
	},
	hint: text{
		en: "If the log says \"Could not get lock\", a background apt run was in progress. Wait a minute and retry. Otherwise check the server's internet access.",
		ru: "Если в логе «Could not get lock», значит шло фоновое обновление apt. Подожди минуту и повтори. Иначе проверь интернет на сервере.",
	},

	apply: func(s *State, _ Options) { s.Docker = true; s.RegistryMirrors = true },

	script: func(o Options) string {
		l := o.Lang()

		// daemon.json собираем через json.Marshal, а не склейкой строк: адрес
		// зеркала приходит снаружи и не должен ломать файл конфигурации.
		cfg, _ := json.MarshalIndent(map[string]any{
			"registry-mirrors": mirrors(o),
			"userland-proxy":   false,
			"dns":              []string{"8.8.8.8", "1.1.1.1"},
		}, "", "  ")

		return `set -e

# On a fresh server a background apt run holds the dpkg lock, so the Docker
# installer (which uses apt too) would fail with "Could not get lock". Wait it out.
i=0
while command -v fuser >/dev/null 2>&1 && fuser \
    /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock \
    /var/lib/apt/lists/lock /var/cache/apt/archives/lock >/dev/null 2>&1; do
  i=$((i+1))
  [ "$i" -gt 60 ] && { echo ` + sq(tr(l,
			"apt lock still held, trying to continue",
			"apt-замок всё ещё занят, пробую продолжить")) + `; break; }
  echo ` + sq(tr(l,
			"waiting for a background apt run (holds the lock)…",
			"жду фоновое обновление apt (держит замок)…")) + `
  sleep 5
done

if ! command -v docker >/dev/null 2>&1; then
  echo ` + sq(tr(l,
			"Docker not found, installing via the official get.docker.com script",
			"Docker не найден, устанавливаю официальным скриптом get.docker.com")) + `
  curl -fsSL https://get.docker.com | sh
else
  echo ` + sq(tr(l,
			"Docker is already installed, skipping installation",
			"Docker уже установлен, пропускаю установку")) + `
fi

mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'JSON'
` + string(cfg) + `
JSON

systemctl restart docker || true
docker compose version >/dev/null 2>&1 || { echo ` + sq(tr(l,
			"docker compose plugin is unavailable",
			"плагин docker compose недоступен")) + `; exit 42; }

echo ` + sq(tr(l,
			"Done: Docker is running, mirrors configured.",
			"Готово: Docker работает, зеркала прописаны.")) + `
`
	},
}

var stepNonRoot = Step{
	Key:     "nonroot",
	Default: false,
	Timeout: 3 * time.Minute,

	title: text{
		en: "Separate user for deployments",
		ru: "Отдельный пользователь для деплоя",
	},
	help: text{
		en: "creates an unprivileged user in the docker group, so you don't work as root",
		ru: "создаёт непривилегированного пользователя в группе docker, чтобы не работать под root",
	},
	hint: text{
		en: "Root access was not modified, you can log in exactly as before.",
		ru: "Root-доступ при этом не менялся, зайти на сервер можно как раньше.",
	},

	apply: func(s *State, o Options) { s.DeployUser = deployUser(o) },

	script: func(o Options) string {
		l := o.Lang()
		user := deployUser(o)

		// Берём переданный ключ, иначе копируем authorized_keys текущего root.
		// Без этого получился бы пользователь, под которого никто не может
		// зайти, и весь шаг оказался бы бессмысленным.
		keySetup := `if [ -f /root/.ssh/authorized_keys ]; then
  cat /root/.ssh/authorized_keys >> "$AK"
  echo ` + sq(tr(l,
			"copied root's keys, log in with: ssh "+user+"@<ip>",
			"скопировал ключи root, заходи: ssh "+user+"@<ip>")) + `
else
  echo ` + sq(tr(l,
			"WARNING: root has no keys, so user "+user+" has no way to authenticate yet.",
			"ВНИМАНИЕ: ключей у root нет, пользователю "+user+" пока нечем авторизоваться.")) + `
  echo ` + sq(tr(l,
			`Add your key: djaploy setup --only nonroot --pubkey "ssh-ed25519 AAAA..."`,
			`Добавь свой ключ: djaploy setup --only nonroot --pubkey "ssh-ed25519 AAAA..."`)) + `
fi`

		if k := strings.TrimSpace(o.PubKey); k != "" {
			keySetup = `grep -qF ` + sq(k) + ` "$AK" || echo ` + sq(k) + ` >> "$AK"
echo ` + sq(tr(l,
				"key added, log in with: ssh "+user+"@<ip>",
				"ключ добавлен, заходи: ssh "+user+"@<ip>"))
		}

		return `set -e
U=` + sq(user) + `

id -u "$U" >/dev/null 2>&1 || useradd -m -s /bin/bash "$U"
getent group docker >/dev/null 2>&1 && usermod -aG docker "$U" || true

mkdir -p "/home/$U/.ssh"
chmod 700 "/home/$U/.ssh"
AK="/home/$U/.ssh/authorized_keys"
touch "$AK"

` + keySetup + `

chmod 600 "$AK"
chown -R "$U:$U" "/home/$U/.ssh"

echo ` + sq(tr(l,
			"Done: user "+user+" created.",
			"Готово: пользователь "+user+" создан.")) + `
echo ` + sq(tr(l,
			"Root access was not modified, we did not touch it.",
			"Root-доступ не менялся, мы его не трогали.")) + `
`
	},
}
