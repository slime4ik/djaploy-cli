package cli

// Version подставляется линкером при сборке релиза (-ldflags).
var Version = "dev"

const helpEN = `djaploy prepares a server for deployment.

Installs and configures what a Dockerised app needs: current packages,
fail2ban, Docker with registry mirrors, a separate user for deployments.

  No account needed. Nothing is sent anywhere.
  Source code: https://github.com/slime4ik/djaploy-cli

COMMANDS

  setup            prepare a server (interactive)
  status           what has already been done on this machine
  update           upgrade djaploy itself (--check only reports)
  uninstall        remove djaploy's files
  version          version

SETUP

  --dry-run        print the scripts and exit, running nothing
  --yes, -y        no questions, default steps
  --only KEYS      only these steps, comma-separated: base,docker,nonroot
  --remote ADDR    run on another server over SSH: user@host[:port]
  --pubkey KEY     public SSH key for the deploy user
  --user NAME      deploy user name (default: deploy)
  --lang CODE      output language: en, ru

EXAMPLES

  See exactly what would run, without running it:
    djaploy setup --dry-run

  Prepare this machine:
    sudo djaploy setup

  Prepare another server from your laptop, with your own SSH key:
    djaploy setup --remote root@203.0.113.10

  Docker only, no questions:
    sudo djaploy setup --only docker --yes

WHAT THIS TOOL DOES NOT DO

  Never touches sshd_config. Never enables ufw. Never removes iptables rules
  it did not create. Sends nothing over the network beyond fetching packages
  from their repositories.
`

const helpRU = `djaploy готовит сервер к деплою.

Ставит и настраивает то, что нужно приложению в Docker: свежие пакеты,
fail2ban, Docker с зеркалами реестра, отдельного пользователя для деплоя.

  Аккаунт не нужен. Ничего никуда не отправляется.
  Исходный код: https://github.com/slime4ik/djaploy-cli

КОМАНДЫ

  setup            подготовить сервер (интерактивно)
  status           что уже сделано на этой машине
  update           обновить сам djaploy (--check только проверит)
  uninstall        убрать файлы djaploy
  version          версия

SETUP

  --dry-run        показать скрипты и выйти, ничего не выполняя
  --yes, -y        без вопросов, шаги по умолчанию
  --only KEYS      только эти шаги через запятую: base,docker,nonroot
  --remote ADDR    выполнить на другом сервере по SSH: user@host[:port]
  --pubkey KEY     публичный SSH-ключ для deploy-пользователя
  --user NAME      имя deploy-пользователя (по умолчанию deploy)
  --lang CODE      язык вывода: en, ru

ПРИМЕРЫ

  Посмотреть, что именно будет выполнено, ничего не запуская:
    djaploy setup --dry-run

  Подготовить эту машину:
    sudo djaploy setup

  Подготовить чужой сервер со своего ноутбука, своим SSH-ключом:
    djaploy setup --remote root@203.0.113.10

  Только Docker, без вопросов:
    sudo djaploy setup --only docker --yes

ЧЕГО ЭТОТ ИНСТРУМЕНТ НЕ ДЕЛАЕТ

  Не трогает sshd_config. Не включает ufw. Не удаляет правила iptables,
  которые создал не он. Не отправляет ничего по сети, кроме скачивания
  пакетов из репозиториев.
`

func helpText() string { return t(helpEN, helpRU) }
