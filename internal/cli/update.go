package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Repo задаёт, откуда берутся релизы. Вынесен в переменную, чтобы форк мог
// пересобрать бинарь со своим адресом через -ldflags.
var Repo = "slime4ik/djaploy-cli"

// maxDownload ограничивает объём, который мы готовы скачать. Бинарь весит
// единицы мегабайт, всё что сильно больше означает, что нам подсунули не то.
const maxDownload = 64 << 20

func runUpdate(args []string) int {
	checkOnly := false
	for _, a := range args {
		switch a {
		case "--check":
			checkOnly = true
		case "--yes", "-y":
			// принимаем ради единообразия с setup, отдельного эффекта нет
		default:
			fail(t("unknown flag %q. Help: djaploy --help",
				"неизвестный флаг «%s». Справка: djaploy --help"), a)
			return 2
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rel, err := latestRelease(ctx)
	if err != nil {
		fail(t("could not check for updates: %v", "не удалось проверить обновления: %v"), err)
		return 1
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(Version, "v")

	say(t("installed: %s, latest: %s", "установлено: %s, доступно: %s"), current, latest)

	if !isNewer(latest, current) {
		say("%s %s", green("✓"), t("Already up to date.", "Уже последняя версия."))
		return 0
	}
	if checkOnly {
		say("%s", t("Run `djaploy update` to install it.",
			"Поставить: `djaploy update`"))
		return 0
	}

	self, err := os.Executable()
	if err != nil {
		fail(t("cannot locate the running binary: %v",
			"не могу найти запущенный бинарь: %v"), err)
		return 1
	}
	self, _ = filepath.EvalSymlinks(self)

	// Проверяем право записи ДО скачивания: обиднее всего упасть после того,
	// как выкачал архив.
	if err := checkWritable(self); err != nil {
		fail(t("no write access to %s: %v", "нет прав на запись в %s: %v"), self, err)
		fail("%s", t("Try: sudo djaploy update", "Попробуй: sudo djaploy update"))
		return 1
	}

	say(t("downloading %s...", "качаю %s..."), latest)
	binary, err := downloadBinary(ctx, rel)
	if err != nil {
		fail(t("download failed: %v", "скачать не удалось: %v"), err)
		return 1
	}

	if err := replaceSelf(self, binary); err != nil {
		fail(t("could not replace the binary: %v", "не удалось заменить бинарь: %v"), err)
		return 1
	}

	say("%s %s", green("✓"), fmt.Sprintf(t("Updated to %s.", "Обновлено до %s."), latest))
	return 0
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func latestRelease(ctx context.Context) (*release, error) {
	url := "https://api.github.com/repos/" + Repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "djaploy/"+Version)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New(t("no releases published yet", "релизов ещё нет"))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API: %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, errors.New(t("release has no tag", "у релиза нет тега"))
	}
	return &rel, nil
}

// downloadBinary качает архив для текущей платформы, сверяет sha256 с
// checksums.txt из того же релиза и достаёт из архива сам бинарь.
//
// Без сверки контрольной суммы обновление было бы дырой: любой, кто сможет
// подменить ответ, получил бы root на сервере.
func downloadBinary(ctx context.Context, rel *release) ([]byte, error) {
	version := strings.TrimPrefix(rel.TagName, "v")
	want := fmt.Sprintf("djaploy_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)

	var archiveURL, sumsURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case want:
			archiveURL = a.URL
		case "checksums.txt":
			sumsURL = a.URL
		}
	}
	if archiveURL == "" {
		return nil, fmt.Errorf(t("no build for %s/%s in release %s",
			"в релизе %[3]s нет сборки под %[1]s/%[2]s"), runtime.GOOS, runtime.GOARCH, rel.TagName)
	}
	if sumsURL == "" {
		return nil, errors.New(t("release has no checksums.txt, refusing to update",
			"в релизе нет checksums.txt, обновляться не буду"))
	}

	sums, err := fetch(ctx, sumsURL)
	if err != nil {
		return nil, fmt.Errorf("checksums.txt: %w", err)
	}
	archive, err := fetch(ctx, archiveURL)
	if err != nil {
		return nil, err
	}

	expected, ok := findSum(string(sums), want)
	if !ok {
		return nil, fmt.Errorf(t("checksums.txt has no entry for %s",
			"в checksums.txt нет строки для %s"), want)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != expected {
		return nil, errors.New(t("checksum mismatch, the download is not what the release says it is",
			"контрольная сумма не совпала, скачано не то, что объявлено в релизе"))
	}

	return extractBinary(archive)
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "djaploy/"+Version)

	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDownload))
}

// findSum разбирает формат `<sha256>  <имя файла>`, как его пишет sha256sum.
func findSum(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			return f[0], true
		}
	}
	return "", false
}

func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || filepath.Base(h.Name) != binaryName() {
			continue
		}
		return io.ReadAll(io.LimitReader(tr, maxDownload))
	}
	return nil, fmt.Errorf(t("archive has no %s", "в архиве нет %s"), binaryName())
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "djaploy.exe"
	}
	return "djaploy"
}

// replaceSelf пишет новый бинарь рядом со старым и переставляет его на место
// одним rename. Так на диске никогда не оказывается наполовину записанного
// файла: либо старая версия, либо новая.
//
// Заменять файл запущенного процесса на unix безопасно: старый inode живёт,
// пока процесс не закончится.
func replaceSelf(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".djaploy-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // сработает только если rename не дошёл

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	mode := os.FileMode(0o755)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// checkWritable проверяет, что мы сможем и создать временный файл рядом, и
// переписать сам бинарь. Одного os.Access мало: rename требует прав на каталог.
func checkWritable(path string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".djaploy-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// isNewer сравнивает версии почисленно, чтобы 0.10.0 считалась новее 0.9.0.
// Версия "dev" (сборка из исходников) обновляется всегда.
func isNewer(latest, current string) bool {
	if current == "dev" || current == "" {
		return true
	}
	l, c := splitVersion(latest), splitVersion(current)
	for i := 0; i < len(l) && i < len(c); i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return len(l) > len(c)
}

func splitVersion(v string) []int {
	// отрезаем суффиксы вроде -rc1: для сравнения хватает числовой части
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}
