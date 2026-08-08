package cli

import (
	"os"
	"path/filepath"

	"github.com/slime4ik/djaploy-cli/provision"
)

// runUninstall убирает следы djaploy.
//
// Осознанно НЕ трогает Docker, fail2ban и deploy-пользователя: на них уже
// может держаться работающий сервер, и снести их за компанию значит сломать
// то, что человек не просил ломать. Вместо этого печатаем, что осталось и
// как убрать это руками.
func runUninstall(args []string) int {
	yes := false
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			yes = true
		}
	}

	if os.Geteuid() != 0 {
		fail(t("Root required: sudo djaploy uninstall",
			"Нужны права root: sudo djaploy uninstall"))
		return 1
	}

	st, _ := provision.LoadState()

	say("%s", bold(t("Will be removed", "Будет удалено")))
	say("  %s  %s", provision.StatePath, t("djaploy state", "состояние djaploy"))
	say("")
	say("%s", bold(t("Will be left alone", "Останется как есть")))
	if st.Docker {
		say("  Docker + /etc/docker/daemon.json  %s", dim(t(
			"your containers run on it",
			"на нём крутятся твои контейнеры")))
	}
	if st.Fail2ban {
		say("  fail2ban + /etc/fail2ban/jail.local  %s", dim("apt-get purge fail2ban"))
	}
	if st.DeployUser != "" {
		say("  %s %s  %s", t("user", "пользователь"), st.DeployUser,
			dim("userdel -r "+st.DeployUser))
	}
	say("")

	if !yes {
		ok, err := confirm(t("Remove djaploy's files?", "Удалить файлы djaploy?"), false)
		if err != nil || !ok {
			say("%s", t("Cancelled.", "Отменено."))
			return 0
		}
	}

	dir := filepath.Dir(provision.StatePath)
	if err := os.RemoveAll(dir); err != nil {
		fail(t("Could not remove %s: %v", "Не удалось удалить %s: %v"), dir, err)
		return 1
	}

	say("%s %s", green("✓"), t("djaploy's files removed.", "Файлы djaploy удалены."))
	say("%s", dim(t("The binary itself: rm ", "Сам бинарь: rm ")+selfPath()))
	return 0
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "$(which djaploy)"
	}
	return p
}
