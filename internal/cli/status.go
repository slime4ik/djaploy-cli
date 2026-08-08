package cli

import (
	"github.com/slime4ik/djaploy-cli/provision"
)

// runStatus показывает состояние ЭТОЙ машины. Сеть и аккаунт не нужны:
// состояние лежит рядом, в /etc/djaploy/state.json.
func runStatus(_ []string) int {
	st, err := provision.LoadState()
	if err != nil {
		fail(t("cannot read %s: %v", "%s не читается: %v"), provision.StatePath, err)
		return 1
	}

	say("%s", bold(t("This machine", "Состояние этой машины")))
	say("")

	mark := func(done bool, title, detail string) {
		box := red("✗")
		if done {
			box = green("✓")
		}
		line := "  " + box + " " + title
		if detail != "" {
			line += "  " + dim(detail)
		}
		say("%s", line)
	}

	mark(st.Fail2ban, "fail2ban", "")
	mark(st.Docker, "Docker", "")
	mark(st.RegistryMirrors, t("registry mirrors", "зеркала реестра"), "")
	mark(st.DeployUser != "", t("deploy user", "deploy-пользователь"), st.DeployUser)

	say("")
	if st.UpdatedAt == "" {
		say("%s", dim(t(
			"Nothing has been configured here yet. Start with: djaploy setup",
			"Здесь ещё ничего не настраивали. Начни с: djaploy setup")))
		return 0
	}
	say("%s", dim(t("updated ", "обновлено ")+st.UpdatedAt+", djaploy "+st.CLIVersion))
	return 0
}
