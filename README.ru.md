<p align="center">
  <img src="docs/gopher.jpg" alt="" width="640">
</p>

<h1 align="center">djaploy</h1>

<p align="center">
  Готовит Linux-сервер к запуску приложения в Docker: свежие пакеты, fail2ban,<br>
  Docker с зеркалами реестра и отдельный непривилегированный пользователь.
</p>

<p align="center"><strong>Без аккаунта. Без регистрации. Ничего никуда не отправляется.</strong></p>

<p align="center">
  <a href="https://github.com/slime4ik/djaploy-cli/actions/workflows/ci.yml"><img src="https://github.com/slime4ik/djaploy-cli/actions/workflows/ci.yml/badge.svg" alt="ci"></a>
  <a href="https://pkg.go.dev/github.com/slime4ik/djaploy-cli"><img src="https://pkg.go.dev/badge/github.com/slime4ik/djaploy-cli.svg" alt="go reference"></a>
  <a href="https://github.com/slime4ik/djaploy-cli/releases/latest"><img src="https://img.shields.io/github/v/release/slime4ik/djaploy-cli?color=blue" alt="release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-yellow.svg" alt="license"></a>
</p>

<p align="center"><a href="README.md">English version</a></p>

---

## Зачем это

[djaploy](https://djaploy.dev) это сервис деплоя. Чтобы настроить сервер, ему
нужен root, а не хотеть отдавать root от своей машины постороннему сервису
нормально.

Поэтому настройку делает этот инструмент. Ты запускаешь его сам, на своём
сервере, и сначала можешь прочитать, что именно он сделает. Это тот же код,
который гоняет сервис, а не упрощённая копия: что проверил, то и выполнится.

На этом можно остановиться. Инструмент полезен сам по себе и таким останется.

## Установка

```sh
curl -fsSL https://raw.githubusercontent.com/slime4ik/djaploy-cli/main/install.sh | sh
```

Если пихать скрипт из интернета сразу в шелл не хочется, это здравая осторожность.
Тогда сначала прочитай:

```sh
curl -fsSL https://raw.githubusercontent.com/slime4ik/djaploy-cli/main/install.sh -o install.sh
less install.sh
sh install.sh
```

В обоих случаях скрипт качает бинарь под твою платформу из
[Releases](https://github.com/slime4ik/djaploy-cli/releases) и сверяет sha256
с `checksums.txt` из того же релиза. Если сумма не совпала, установка
прерывается. Для установки root не нужен, он нужен команде `setup`.

Другие способы: скачать из Releases руками или `go install
github.com/slime4ik/djaploy-cli/cmd/djaploy@latest`.

Дальше `djaploy update` обновляет на месте с той же проверкой суммы.

## Как пользоваться

Посмотреть, что произойдёт, ничего не выполняя:

```sh
djaploy setup --dry-run
```

Печатает сами shell-скрипты, а не их пересказ. Прочитал, запускай:

```sh
sudo djaploy setup
```

Появится меню, отметишь что нужно. Или без меню:

```sh
sudo djaploy setup --only docker --yes
```

Со своего ноутбука, по SSH, своим ключом:

```sh
djaploy setup --remote root@203.0.113.10
```

Что уже настроено:

```sh
djaploy status
```

`djaploy setup` без флагов спросит всё сам: шаги, имя deploy-пользователя и
подтверждение. Флаги нужны для скриптов. Полный список: `djaploy --help`.

Вывод по умолчанию английский на любой машине. Русский: `--lang ru` или
`DJAPLOY_LANG=ru`. Системная локаль намеренно не учитывается, чтобы инструмент
везде выглядел одинаково.

## Что он делает

| Шаг       | Что меняет                                                            |
|-----------|-----------------------------------------------------------------------|
| `base`    | `apt update`, ставит `fail2ban`, `curl`, `ca-certificates`, `git`; пишет `/etc/fail2ban/jail.local` (SSH: 5 попыток, бан на час) |
| `docker`  | ставит Docker официальным скриптом `get.docker.com`, если его нет; пишет `/etc/docker/daemon.json` с зеркалами реестра |
| `nonroot` | создаёт пользователя в группе `docker` и кладёт ему `authorized_keys` |

Состояние пишется в `/etc/djaploy/state.json`, на твоей машине, читаемо тобой,
используется командой `djaploy status`. Никуда не выгружается.

## Чего он намеренно не делает

- **Никогда не трогает `sshd_config`.** Сломанный sshd это сервер, до которого
  ты больше не достучишься.
- **Никогда не включает `ufw`.** Если ufw уже активен, откроет 80/443 и всё.
  Включить фаервол за человека, который не просил, значит рискнуть отрезать ему
  текущую сессию.
- **Не удаляет правила iptables, которые создал не он.** Учти: настройка Docker
  означает его перезапуск, а Docker на каждом старте перестраивает свои цепочки
  (`DOCKER`, `DOCKER-USER`, маскарадинг для `docker0`). Ими владеет он сам,
  остальное не трогается.
- **Ничего не шлёт по сети**, кроме скачивания пакетов из репозиториев и
  проверки новой версии на GitHub по команде `update`.

Это проверяется тестами, а не только обещанием, см.
[`provision/steps_test.go`](provision/steps_test.go).

Сверх того [CI](.github/workflows/ci.yml) гоняет все шаги на чистой Ubuntu и
сверяет обещания со слепком, снятым до прогона: `sshd_config` не изменился ни на
байт, ufw в том же состоянии, ни одно уже существовавшее правило iptables не
пропало, второй прогон ничего не меняет. Бейдж наверху это она и есть.

Каждый шаг идемпотентен: запустить дважды безопасно.

## Как убрать

```sh
sudo djaploy uninstall
```

Удаляет `/etc/djaploy` и печатает, что осталось (Docker, fail2ban,
deploy-пользователь) вместе с командой, которой каждое можно снести самому. Сам
он их не тронет: на них, скорее всего, крутятся твои контейнеры.

## Связка с djaploy (по желанию)

Пока не сделана. Когда появится, это будет вход по коду устройства: CLI
показывает код, ты подтверждаешь его в браузере, где уже залогинен, и никакой
секрет в терминале не набирается. До тех пор инструмент самостоятельный.

## Заметки

- Рассчитан на Ubuntu и Debian. Другие дистрибутивы пока не поддерживаются.
- `--remote` проверяет ключ сервера по `~/.ssh/known_hosts` и спрашивает
  подтверждение на незнакомый сервер, так же как это делает `ssh`. Изменившийся
  ключ это всегда ошибка.
- Шаг `docker` выполняет `curl https://get.docker.com | sh`. Это официальный
  установщик Docker, переписывать его мы не стали, но знать об этом стоит.
- Релизы проверяются по sha256, но не подписаны. Подпись это разумный следующий
  шаг, пока не сделано.

## Лицензия

MIT, см. [LICENSE](LICENSE).
