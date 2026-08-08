package provision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Remote выполняет скрипты на другом сервере по SSH, это режим `--remote`.
//
// Работает с ноутбука на твоих собственных ключах. Серверы djaploy в этом
// режиме не участвуют вообще, ни один секрет никуда не уходит.
type Remote struct {
	client *ssh.Client
	addr   string // user@host для вывода
	sudoPw string // пароль для sudo; пусто = зашли под root
}

// RemoteConfig описывает, куда и как подключаться.
type RemoteConfig struct {
	User string
	Host string
	Port string

	// Password: если задан, пробуем пароль. Иначе только ключи и агент.
	Password string

	// HostKeyPrompt спрашивает про незнакомый сервер. Вернуть true = доверяем и
	// дописываем в known_hosts. nil = незнакомый сервер отвергаем.
	HostKeyPrompt func(host, fingerprint string) (bool, error)

	// SudoPassword запрашивается лениво, только если вход был не под root.
	SudoPassword func() (string, error)
}

func Dial(ctx context.Context, cfg RemoteConfig) (*Remote, error) {
	if cfg.Port == "" {
		cfg.Port = "22"
	}
	if cfg.User == "" {
		cfg.User = "root"
	}

	hostKey, err := hostKeyCallback(cfg.HostKeyPrompt)
	if err != nil {
		return nil, err
	}

	auth := authMethods(cfg.Password)
	if len(auth) == 0 {
		return nil, fmt.Errorf("нет способа авторизации: не найден ssh-агент и ключи в ~/.ssh, пароль не задан")
	}

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	d := net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("не могу подключиться к %s: %w", addr, err)
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         15 * time.Second,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH к %s@%s не прошёл: %w", cfg.User, cfg.Host, err)
	}

	r := &Remote{client: ssh.NewClient(c, chans, reqs), addr: cfg.User + "@" + cfg.Host}

	// если вошли не под root, понадобится sudo; пароль спрашиваем один раз
	if strings.TrimSpace(r.capture(ctx, "id -u")) != "0" {
		if cfg.SudoPassword == nil {
			r.Close()
			return nil, fmt.Errorf("вход не под root, а пароль для sudo взять неоткуда")
		}
		pw, err := cfg.SudoPassword()
		if err != nil {
			r.Close()
			return nil, err
		}
		r.sudoPw = pw
		if strings.TrimSpace(r.capture(ctx, r.wrap("id -u"))) != "0" {
			r.Close()
			return nil, fmt.Errorf("sudo для «%s» не сработал. Проверь пароль и что пользователь в группе sudo", cfg.User)
		}
	}
	return r, nil
}

func (r *Remote) Target() string { return r.addr }

func (r *Remote) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// wrap под root отдаёт скрипт как есть, иначе заворачивает его в sudo.
//
// Пароль уходит в stdin (`sudo -S`), а не в argv. В отличие от строки вида
// `printf pass | sudo -S ...` он не виден в `ps` другим процессам сервера.
func (r *Remote) wrap(script string) string {
	if r.sudoPw == "" {
		return script
	}
	return "sudo -S -p '' bash -s"
}

func (r *Remote) Run(ctx context.Context, script string, out func(string)) error {
	sess, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	// под sudo первой строкой stdin идёт пароль (его съедает sudo -S),
	// дальше идёт сам скрипт, его читает bash -s
	stdin := script
	cmd := "bash -s"
	if r.sudoPw != "" {
		stdin = r.sudoPw + "\n" + script
		cmd = "sudo -S -p '' bash -s"
	}
	sess.Stdin = strings.NewReader(stdin)

	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return err
	}
	if err := sess.Start(cmd); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go streamLines(&wg, stdout, out)
	go streamLines(&wg, stderr, out)

	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()

	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		_ = sess.Close()
		return ctx.Err()
	case err := <-done:
		wg.Wait()
		return err
	}
}

// capture выполняет команду и собирает вывод, для мелких проверок вроде `id -u`.
func (r *Remote) capture(ctx context.Context, cmd string) string {
	sess, err := r.client.NewSession()
	if err != nil {
		return ""
	}
	defer sess.Close()

	if r.sudoPw != "" && strings.HasPrefix(cmd, "sudo") {
		sess.Stdin = strings.NewReader(r.sudoPw + "\n")
	}
	var sb strings.Builder
	sess.Stdout = &sb

	done := make(chan struct{})
	go func() { _ = sess.Run(cmd); close(done) }()

	select {
	case <-ctx.Done():
		_ = sess.Close()
		return ""
	case <-done:
		return sb.String()
	}
}

// authMethods пробует ssh-агент, потом стандартные ключи, потом пароль.
// Тот же порядок, что у обычного ssh, чтобы не удивлять.
func authMethods(password string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	home, _ := os.UserHomeDir()
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		raw, err := os.ReadFile(filepath.Join(home, ".ssh", name))
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(raw)
		if err != nil {
			continue // ключ под паролем, этот случай закрывает ssh-агент
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if password != "" {
		methods = append(methods, ssh.Password(password))
		// многие серверы с PAM принимают только keyboard-interactive
		methods = append(methods, ssh.KeyboardInteractive(
			func(_, _ string, qs []string, _ []bool) ([]string, error) {
				ans := make([]string, len(qs))
				for i := range ans {
					ans[i] = password
				}
				return ans, nil
			}))
	}
	return methods
}

// hostKeyCallback делает настоящую проверку по ~/.ssh/known_hosts.
//
// Незнакомый сервер не пропускаем молча: спрашиваем и показываем отпечаток,
// как это делает ssh. Инструмент про доверие не может игнорировать MITM.
func hostKeyCallback(prompt func(string, string) (bool, error)) (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".ssh", "known_hosts")

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		f.Close()
	}

	known, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("не читается %s: %w", path, err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := known(hostname, remote, key)
		if err == nil {
			return nil
		}

		var kerr *knownhosts.KeyError
		// KeyError с непустым Want = ключ известен, но ДРУГОЙ. Это либо смена
		// ключа на сервере, либо MITM. Молча принимать нельзя ни в каком случае.
		if errors.As(err, &kerr) && len(kerr.Want) > 0 {
			return fmt.Errorf(
				"ключ сервера %s не совпадает с записью в known_hosts.\n"+
					"Это бывает при переустановке сервера, но так же выглядит и перехват.\n"+
					"Если сервер переставляли: ssh-keygen -R %s", hostname, hostname)
		}

		if prompt == nil {
			return fmt.Errorf("сервер %s неизвестен, а подтвердить некому", hostname)
		}
		accept, err := prompt(hostname, ssh.FingerprintSHA256(key))
		if err != nil {
			return err
		}
		if !accept {
			return fmt.Errorf("подключение отменено: ключ сервера не подтверждён")
		}
		return appendKnownHost(path, hostname, key)
	}, nil
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	_, err = io.WriteString(f, line+"\n")
	return err
}
