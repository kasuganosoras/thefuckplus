#!/usr/bin/env bash
# Install thefuckplus binary + wire ZSH plugin into ~/.zshrc
set -euo pipefail

REPO_URL="${THEFUCKPLUS_REPO:-https://github.com/kasuganosoras/thefuckplus.git}"
INSTALL_DIR="${THEFUCKPLUS_INSTALL_DIR:-$HOME/.local/share/thefuckplus}"
BIN_DIR="${THEFUCKPLUS_BIN_DIR:-$HOME/.local/bin}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

info() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

detect_target() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "unsupported arch: $arch" ;;
  esac
  case "$os" in
    linux|darwin) ;;
    *) die "unsupported os: $os" ;;
  esac
  echo "${os}_${arch}"
}

install_from_source() {
  need_cmd go
  info "Building from source..."
  mkdir -p "$BIN_DIR"
  (
    cd "$REPO_ROOT"
    go build -o "$BIN_DIR/thefuckplus" ./cmd/thefuckplus
  )
  info "Installed binary: $BIN_DIR/thefuckplus"
}

install_plugin_files() {
  info "Installing plugin files to $INSTALL_DIR"
  mkdir -p "$INSTALL_DIR/shell" "$INSTALL_DIR/scripts"
  cp -f "$REPO_ROOT/thefuckplus.plugin.zsh" "$INSTALL_DIR/"
  cp -f "$REPO_ROOT/shell/thefuckplus.plugin.zsh" "$INSTALL_DIR/shell/"
  # Keep wrapper working when copied out of repo
  cat > "$INSTALL_DIR/thefuckplus.plugin.zsh" <<'EOF'
0="${${ZERO:-${0:#$ZSH_ARGZERO}}:-${(%):-%N}}"
0="${${(M)0:#/*}:-$PWD/$0}"
source "${0:A:h}/shell/thefuckplus.plugin.zsh"
EOF
}

ensure_path_line() {
  local zshrc="$HOME/.zshrc"
  local path_line='export PATH="$HOME/.local/bin:$PATH"'
  touch "$zshrc"
  if ! grep -qF '.local/bin' "$zshrc" 2>/dev/null; then
    info "Adding ~/.local/bin to PATH in ~/.zshrc"
    printf '\n# thefuckplus\n%s\n' "$path_line" >>"$zshrc"
  fi
}

ensure_source_line() {
  local zshrc="$HOME/.zshrc"
  local marker="# thefuckplus plugin"
  local source_line="source \"$INSTALL_DIR/thefuckplus.plugin.zsh\""
  touch "$zshrc"
  if grep -qF "$INSTALL_DIR/thefuckplus.plugin.zsh" "$zshrc" 2>/dev/null; then
    info "Plugin already sourced in ~/.zshrc"
    return
  fi
  info "Appending plugin source to ~/.zshrc"
  printf '\n%s\n%s\n' "$marker" "$source_line" >>"$zshrc"
}

main() {
  info "thefuckplus installer"
  detect_target >/dev/null

  # Prefer local source build; release download can be added when tags exist.
  if [[ -f "$REPO_ROOT/go.mod" && -d "$REPO_ROOT/cmd/thefuckplus" ]]; then
    install_from_source
  elif command -v go >/dev/null 2>&1; then
    info "Installing via go install..."
    mkdir -p "$BIN_DIR"
    GOBIN="$BIN_DIR" go install github.com/kasuganosoras/thefuckplus/cmd/thefuckplus@latest
  else
    die "Go toolchain not found; install Go or build from a source checkout"
  fi

  install_plugin_files
  ensure_path_line
  ensure_source_line

  info "Done."
  echo
  echo "Next steps:"
  echo "  1) Restart ZSH or: source ~/.zshrc"
  echo "  2) Configure AI:  thefuckplus configure"
  echo "  3) Try:           fuck"
  echo "     or:            fuck find files larger than 100mb"
}

main "$@"
