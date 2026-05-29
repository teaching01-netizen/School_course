#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Load local env (gitignored) if present.
if [ -f "$ROOT_DIR/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

GO_VERSION="${GO_VERSION:-1.25.7}"
NODE_VERSION="${NODE_VERSION:-22.16.0}"

TOOLS_DIR="$ROOT_DIR/.tools"
GO_DIR="$TOOLS_DIR/go"
NODE_DIR="$TOOLS_DIR/node"

install_go() {
  if [ -x "$GO_DIR/bin/go" ]; then
    return
  fi
  mkdir -p "$TOOLS_DIR"
  local tgz="go${GO_VERSION}.darwin-arm64.tar.gz"
  local url="https://go.dev/dl/${tgz}"
  echo "Installing Go ${GO_VERSION} -> ${GO_DIR}"
  curl -fsSL "$url" -o "$TOOLS_DIR/$tgz"
  rm -rf "$GO_DIR" "$TOOLS_DIR/go-tmp" "$TOOLS_DIR/go"
  tar -C "$TOOLS_DIR" -xzf "$TOOLS_DIR/$tgz"
  mv "$TOOLS_DIR/go" "$GO_DIR"
}

install_node() {
  if [ -x "$NODE_DIR/bin/node" ]; then
    return
  fi
  mkdir -p "$TOOLS_DIR"
  local tgz="node-v${NODE_VERSION}-darwin-arm64.tar.gz"
  local url="https://nodejs.org/dist/v${NODE_VERSION}/${tgz}"
  echo "Installing Node ${NODE_VERSION} -> ${NODE_DIR}"
  curl -fsSL "$url" -o "$TOOLS_DIR/$tgz"
  rm -rf "$NODE_DIR" "$TOOLS_DIR/node-tmp"
  mkdir -p "$TOOLS_DIR/node-tmp"
  tar -C "$TOOLS_DIR/node-tmp" -xzf "$TOOLS_DIR/$tgz" --strip-components=1
  mv "$TOOLS_DIR/node-tmp" "$NODE_DIR"
}

install_go
install_node

export PATH="$NODE_DIR/bin:$GO_DIR/bin:$PATH"

echo "node: $(node -v)"
echo "npm:  $(npm -v)"
echo "go:   $(go version)"

echo
echo "== Frontend build =="
npm ci
npm run build

echo
echo "== Backend unit tests =="
(cd backend && go test ./...)

if [ -n "${TEST_DATABASE_URL:-}" ]; then
  echo
  echo "== Backend DB integration tests (TEST_DATABASE_URL set) =="
  (cd backend && go test ./internal/db -run TestStudent -count=1)
else
  echo
  echo "== Skipping DB integration tests (set TEST_DATABASE_URL) =="
fi
