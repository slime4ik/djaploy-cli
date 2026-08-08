<p align="center">
  <img src="docs/gopher.jpg" alt="" width="640">
</p>

<h1 align="center">djaploy</h1>

<p align="center">
  Prepares a Linux server for running a Dockerised app: current packages,<br>
  fail2ban, Docker with registry mirrors, and a separate unprivileged user.
</p>

<p align="center"><strong>No account. No signup. Nothing is sent anywhere.</strong></p>

<p align="center">
  <a href="https://github.com/slime4ik/djaploy-cli/actions/workflows/ci.yml"><img src="https://github.com/slime4ik/djaploy-cli/actions/workflows/ci.yml/badge.svg" alt="ci"></a>
  <a href="https://pkg.go.dev/github.com/slime4ik/djaploy-cli"><img src="https://pkg.go.dev/badge/github.com/slime4ik/djaploy-cli.svg" alt="go reference"></a>
  <a href="https://github.com/slime4ik/djaploy-cli/releases/latest"><img src="https://img.shields.io/github/v/release/slime4ik/djaploy-cli?color=blue" alt="release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-yellow.svg" alt="license"></a>
</p>

<p align="center"><a href="README.ru.md">Русская версия</a></p>

---

## Why this exists

[djaploy](https://djaploy.dev) is a managed-deployment service. To set up a
server it needs root access, and refusing to hand root over to a stranger is a
reasonable position.

So the setup part is this tool instead. You run it yourself, on your own server,
and you can read exactly what it will do first. It is the same code the service
runs, not a simplified copy, so what you audit here is what actually happens.

You can stop after this. The tool is useful on its own and will stay that way.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/slime4ik/djaploy-cli/main/install.sh | sh
```

If piping a script into a shell makes you uncomfortable, good. Read it first:

```sh
curl -fsSL https://raw.githubusercontent.com/slime4ik/djaploy-cli/main/install.sh -o install.sh
less install.sh
sh install.sh
```

Either way the script downloads the binary for your platform from
[Releases](https://github.com/slime4ik/djaploy-cli/releases) and verifies its
sha256 against `checksums.txt` from the same release. A mismatch aborts the
install. Installing does not need root; `setup` does.

Other ways: download from Releases by hand, or `go install
github.com/slime4ik/djaploy-cli/cmd/djaploy@latest`.

Later, `djaploy update` upgrades in place with the same checksum check.

## Use

See what would happen, without running anything:

```sh
djaploy setup --dry-run
```

This prints the actual shell scripts, not a summary of them. Read them, then:

```sh
sudo djaploy setup
```

You get a menu; pick what you want. Or skip the menu:

```sh
sudo djaploy setup --only docker --yes
```

From your laptop, over SSH, using your own key:

```sh
djaploy setup --remote root@203.0.113.10
```

Check what is configured:

```sh
djaploy status
```

`djaploy setup` with no flags asks for everything: steps, the deploy user name,
and a confirmation. Flags are there for scripts. Full list: `djaploy --help`.

Output is English by default on every machine. For Russian: `--lang ru` or
`DJAPLOY_LANG=ru`. The system locale is deliberately ignored, so the tool reads
the same everywhere.

## What it does

| Step      | What it changes                                                       |
|-----------|-----------------------------------------------------------------------|
| `base`    | `apt update`, installs `fail2ban`, `curl`, `ca-certificates`, `git`; writes `/etc/fail2ban/jail.local` (SSH: 5 tries, 1 hour ban) |
| `docker`  | installs Docker via the official `get.docker.com` script if absent; writes `/etc/docker/daemon.json` with registry mirrors |
| `nonroot` | creates a user in the `docker` group and gives it an `authorized_keys` file |

State is written to `/etc/djaploy/state.json`, on your machine, readable by you,
used by `djaploy status`. It is not uploaded anywhere.

## What it deliberately does not do

- **Never touches `sshd_config`.** A broken sshd means a server you cannot reach.
- **Never enables `ufw`.** If ufw is already active it opens 80/443 and nothing
  more. Enabling a firewall for someone who did not ask can cut their session.
- **Never removes iptables rules it did not create.** Note that configuring
  Docker means restarting it, and Docker rebuilds its own chains (`DOCKER`,
  `DOCKER-USER`, masquerading for `docker0`) on every start. Those are Docker's
  to manage; everything else is left alone.
- **Sends nothing over the network** beyond fetching packages from their
  repositories, and checking GitHub for a new version when you run `update`.

These are enforced by tests, not just by intent, in
[`provision/steps_test.go`](provision/steps_test.go).

On top of that, [CI](.github/workflows/ci.yml) runs every step on a clean Ubuntu
and then verifies the promises against a snapshot taken beforehand: `sshd_config`
is byte-for-byte unchanged, ufw is in the same state it started in, no
pre-existing iptables rule disappeared, and a second run changes nothing. The
badge above is that check.

Every step is idempotent: running it twice is safe.

## Removing it

```sh
sudo djaploy uninstall
```

Removes `/etc/djaploy` and prints what is left behind (Docker, fail2ban, the
deploy user) with the command to remove each one yourself. It will not delete
those for you: your containers are probably running on them.

## Connecting it to djaploy (optional)

Not implemented yet. When it is, it will be a device-code login: the CLI shows a
code, you approve it in a browser where you are already signed in, and no secret
is ever typed into a terminal. Until then this tool is standalone.

## Notes

- Targets Ubuntu and Debian. Other distributions are not supported yet.
- `--remote` verifies the host key against `~/.ssh/known_hosts` and asks before
  trusting a new server, the way `ssh` does. A changed key is always an error.
- The `docker` step runs `curl https://get.docker.com | sh`. That is Docker's own
  official installer. We did not want to reimplement it, but it is worth knowing.
- Releases are verified by sha256, not signed. Signature verification is a
  reasonable next step and is not done yet.

## Licence

MIT, see [LICENSE](LICENSE).
