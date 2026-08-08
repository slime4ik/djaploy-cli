package provision

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// StatePath указывает, где на сервере лежит состояние. Источник правды живёт
// на самой машине, а не в чужой базе, поэтому `djaploy status` работает без
// сети и без аккаунта.
const StatePath = "/etc/djaploy/state.json"

// State описывает, что уже сделано на сервере. Имена полей совпадают с
// ServerState на бэкенде djaploy, чтобы состояние читалось панелью без
// конвертации.
type State struct {
	Fail2ban        bool   `json:"fail2ban"`
	Docker          bool   `json:"docker"`
	RegistryMirrors bool   `json:"registry_mirrors"`
	DeployUser      string `json:"deploy_user"`
	UpdatedAt       string `json:"updated_at"`
	CLIVersion      string `json:"cli_version"`
}

// LoadState читает состояние с этой машины. Если файла нет, возвращает пустое
// состояние без ошибки: значит, здесь ещё ничего не делали.
func LoadState() (*State, error) {
	raw, err := os.ReadFile(StatePath)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		// битый файл не должен блокировать работу, начинаем с чистого
		return &State{}, nil
	}
	return &s, nil
}

// ReadState читает состояние там, где работает Runner: локально или на
// удалённом сервере. Если файла нет или он не разобрался, вернёт пустое.
func ReadState(ctx context.Context, r Runner) *State {
	var buf []byte
	script := "cat " + sq(StatePath) + " 2>/dev/null || true"
	if err := r.Run(ctx, script, func(line string) {
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}); err != nil {
		return &State{}
	}
	var s State
	if err := json.Unmarshal(buf, &s); err != nil {
		return &State{}
	}
	return &s
}

// Save записывает состояние через Runner, то есть туда, где мы работали:
// локально или на удалённый сервер по SSH.
func (s *State) Save(ctx context.Context, r Runner, version string) error {
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.CLIVersion = version

	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// кавычки в метке heredoc не дают shell трогать содержимое JSON
	script := "set -e\nmkdir -p " + sq(filepath.Dir(StatePath)) +
		"\ncat > " + sq(StatePath) + " <<'DJAPLOY_STATE'\n" + string(raw) +
		"\nDJAPLOY_STATE\nchmod 644 " + sq(StatePath) + "\n"

	return r.Run(ctx, script, func(string) {})
}
