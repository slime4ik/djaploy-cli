//go:build unix

package provision

import (
	"os"
	"os/exec"
	"syscall"
)

func checkLocalSupported() error {
	if os.Geteuid() != 0 {
		return ErrNeedRoot
	}
	return nil
}

// setupProcessGroup сажает bash в отдельную группу процессов и при отмене бьёт
// по всей группе. Обычный kill достал бы только сам bash, а порождённые им
// apt, dpkg и docker продолжили бы работать с открытыми пайпами.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// минус перед pid означает "вся группа"
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
