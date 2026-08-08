package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.2.0", "0.1.0", true},
		{"0.1.0", "0.2.0", false},
		{"0.1.0", "0.1.0", false},
		{"1.0.0", "0.9.9", true},
		{"0.10.0", "0.9.0", true},  // почисленно, а не по алфавиту
		{"0.9.0", "0.10.0", false}, // строковое сравнение дало бы true
		{"0.1.1", "0.1", true},
		{"0.2.0", "dev", true}, // сборка из исходников обновляется всегда
		{"0.2.0", "", true},
		{"0.2.0-rc1", "0.1.0", true}, // суффикс не мешает разбору
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, ожидалось %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestFindSum(t *testing.T) {
	sums := "aaa111  djaploy_0.1.0_linux_amd64.tar.gz\n" +
		"bbb222  djaploy_0.1.0_linux_arm64.tar.gz\n"

	if got, ok := findSum(sums, "djaploy_0.1.0_linux_arm64.tar.gz"); !ok || got != "bbb222" {
		t.Errorf("findSum вернул %q/%v", got, ok)
	}
	if _, ok := findSum(sums, "djaploy_0.1.0_darwin_amd64.tar.gz"); ok {
		t.Error("findSum нашёл отсутствующий файл")
	}
}

func TestExtractBinary(t *testing.T) {
	want := []byte("не настоящий бинарь, но сойдёт")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// в архиве есть и лишние файлы, извлечь надо именно бинарь
	for _, f := range []struct {
		name string
		body []byte
	}{
		{"README.md", []byte("readme")},
		{binaryName(), want},
	} {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.body); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()

	got, err := extractBinary(buf.Bytes())
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("извлекли %q, ожидалось %q", got, want)
	}
}

func TestExtractBinaryFailsWithoutBinary(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "README.md", Size: 0, Typeflag: tar.TypeReg})
	tw.Close()
	gz.Close()

	if _, err := extractBinary(buf.Bytes()); err == nil {
		t.Error("архив без бинаря принят молча")
	}
}

// Замена должна быть атомарной: либо старый файл, либо новый, без огрызков.
func TestReplaceSelfKeepsModeAndSwapsAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "djaploy")

	if err := os.WriteFile(path, []byte("старый"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceSelf(path, []byte("новый")); err != nil {
		t.Fatalf("replaceSelf: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "новый" {
		t.Errorf("содержимое %q, ожидалось «новый»", got)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Errorf("права стали %v, ожидались 0755", st.Mode().Perm())
	}

	// временные файлы за собой не оставляем
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("в каталоге осталось %d файлов, ожидался 1", len(entries))
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "djaploy")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkWritable(path); err != nil {
		t.Errorf("записываемый каталог признан недоступным: %v", err)
	}
	// после проверки не должно остаться следов
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Error("checkWritable оставил за собой временный файл")
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skip("не удалось снять права на запись")
	}
	defer os.Chmod(dir, 0o700)

	if os.Geteuid() == 0 {
		t.Skip("под root права каталога не ограничивают")
	}
	if err := checkWritable(path); err == nil {
		t.Error("каталог только для чтения признан записываемым")
	}
}
