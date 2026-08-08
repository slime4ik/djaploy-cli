//go:build windows

package provision

import (
	"errors"
	"os/exec"
)

// На Windows готовить нечего: шаги ставят apt-пакеты и Docker для Linux.
// Собираться под Windows модуль всё же должен, потому что оттуда работает
// `--remote`, то есть подготовка чужого Linux-сервера по SSH.
func checkLocalSupported() error {
	return errors.New("локальный режим доступен только на Linux и macOS; с Windows используй --remote user@host")
}

func setupProcessGroup(*exec.Cmd) {}
