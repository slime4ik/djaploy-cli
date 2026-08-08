#!/bin/sh
# Установщик djaploy.
#
# Скачивает бинарь под твою платформу из GitHub Releases и сверяет sha256
# с checksums.txt из того же релиза. Без совпадения контрольной суммы
# установка прерывается.
#
# Root не нужен: если /usr/local/bin недоступен для записи, ставим в
# ~/.local/bin. Права root нужны потом, самой команде `djaploy setup`.
#
#   sh install.sh                     последняя версия
#   DJAPLOY_VERSION=0.2.0 sh install.sh   конкретная версия
#   DJAPLOY_INSTALL_DIR=~/bin sh install.sh
#
# Прочитать перед запуском это нормально и правильно:
#   curl -fsSL https://raw.githubusercontent.com/slime4ik/djaploy-cli/main/install.sh -o install.sh
#   less install.sh && sh install.sh

set -eu

REPO="slime4ik/djaploy-cli"
VERSION="${DJAPLOY_VERSION:-}"
INSTALL_DIR="${DJAPLOY_INSTALL_DIR:-}"

die() { printf '%s\n' "ошибка: $*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

need() { command -v "$1" >/dev/null 2>&1; }

# --- что качать ---------------------------------------------------------

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) die "неподдерживаемая система: $os. djaploy готовит Linux-серверы; с других систем работает режим --remote." ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "неподдерживаемая архитектура: $arch" ;;
esac

# --- чем качать и чем проверять -----------------------------------------

if need curl; then
  fetch() { curl -fsSL "$1"; }
  fetch_to() { curl -fsSL "$1" -o "$2"; }
elif need wget; then
  fetch() { wget -qO- "$1"; }
  fetch_to() { wget -qO "$2" "$1"; }
else
  die "нужен curl или wget"
fi

# Контрольную сумму не пропускаем: без неё установка теряет смысл.
if need sha256sum; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif need shasum; then
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  die "нужен sha256sum или shasum, иначе нечем проверить скачанное"
fi

need tar || die "нужен tar"

# --- версия --------------------------------------------------------------

if [ -z "$VERSION" ]; then
  info "узнаю последнюю версию..."
  VERSION=$(fetch "https://api.github.com/repos/$REPO/releases/latest" \
    | tr ',' '\n' | grep '"tag_name"' | head -1 | cut -d'"' -f4 || true)
  [ -n "$VERSION" ] || die "не удалось узнать последнюю версию. Задай явно: DJAPLOY_VERSION=0.1.0"
fi
VERSION="${VERSION#v}"

# --- куда ставить --------------------------------------------------------

if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ] 2>/dev/null; then
    INSTALL_DIR=/usr/local/bin
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi
mkdir -p "$INSTALL_DIR" || die "не могу создать $INSTALL_DIR"
[ -w "$INSTALL_DIR" ] || die "нет прав на запись в $INSTALL_DIR. Задай другой: DJAPLOY_INSTALL_DIR=~/bin"

# --- качаем и проверяем ---------------------------------------------------

archive="djaploy_${VERSION}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/v${VERSION}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

info "качаю djaploy $VERSION ($os/$arch)"
fetch_to "$base/$archive" "$tmp/$archive" || die "не скачался $base/$archive"
fetch_to "$base/checksums.txt" "$tmp/checksums.txt" || die "не скачался checksums.txt"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1 || true)
[ -n "$expected" ] || die "в checksums.txt нет строки для $archive"

actual=$(sha256 "$tmp/$archive")
if [ "$expected" != "$actual" ]; then
  die "контрольная сумма не совпала.
  ожидалась: $expected
  получена:  $actual
Скачано не то, что объявлено в релизе. Установка прервана."
fi
info "контрольная сумма совпала"

tar -xzf "$tmp/$archive" -C "$tmp" djaploy || die "не удалось распаковать архив"
chmod +x "$tmp/djaploy"
mv "$tmp/djaploy" "$INSTALL_DIR/djaploy" || die "не удалось положить бинарь в $INSTALL_DIR"

# --- что дальше -----------------------------------------------------------

info ""
info "готово: $INSTALL_DIR/djaploy"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    info ""
    info "$INSTALL_DIR не в PATH. Добавь в ~/.profile или ~/.bashrc:"
    info "  export PATH=\"\$PATH:$INSTALL_DIR\""
    ;;
esac

info ""
info "Посмотреть, что он сделает с сервером, ничего не запуская:"
info "  djaploy setup --dry-run"
info ""
info "Подготовить сервер:"
info "  sudo djaploy setup"
