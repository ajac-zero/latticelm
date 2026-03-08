#!/usr/bin/env bash
# Setup script for LatticeLM development on a fresh Ubuntu VM.
# Usage: sudo bash scripts/setup-ubuntu.sh
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "Error: this script must be run as root (use sudo)." >&2
  exit 1
fi

echo "==> Updating system packages..."
apt-get update && apt-get upgrade -y

echo "==> Installing base build tools..."
apt-get install -y \
  build-essential \
  gcc \
  git \
  curl \
  wget \
  ca-certificates \
  gnupg \
  make \
  entr

# ---------- Go 1.26.1 ----------
GO_VERSION="1.26.1"
if command -v go &>/dev/null && go version | grep -q "go${GO_VERSION}"; then
  echo "==> Go ${GO_VERSION} already installed, skipping."
else
  echo "==> Installing Go ${GO_VERSION}..."
  ARCH=$(dpkg --print-architecture)
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" -o /tmp/go.tar.gz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tar.gz
  rm /tmp/go.tar.gz
fi

# Make Go available for the rest of this script and future logins
export PATH="/usr/local/go/bin:${PATH}"
if ! grep -q '/usr/local/go/bin' /etc/profile.d/go.sh 2>/dev/null; then
  echo 'export PATH="/usr/local/go/bin:${HOME}/go/bin:${PATH}"' > /etc/profile.d/go.sh
fi

# ---------- Node.js 18 LTS (for frontend) ----------
if command -v node &>/dev/null; then
  echo "==> Node $(node -v) already installed, skipping."
else
  echo "==> Installing Node.js 18 LTS..."
  curl -fsSL https://deb.nodesource.com/setup_18.x | bash -
  apt-get install -y nodejs
fi

# ---------- Docker & Docker Compose ----------
if command -v docker &>/dev/null; then
  echo "==> Docker already installed, skipping."
else
  echo "==> Installing Docker..."
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "${VERSION_CODENAME}") stable" | \
    tee /etc/apt/sources.list.d/docker.list > /dev/null
  apt-get update
  apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi

# ---------- SQLite dev headers (CGO dependency for go-sqlite3) ----------
echo "==> Installing SQLite development libraries..."
apt-get install -y libsqlite3-dev

# ---------- Optional Go dev tools ----------
echo "==> Installing Go linter and security scanner..."
GOBIN="/usr/local/go/bin"
GOPATH_TMP=$(mktemp -d)
GOPATH="${GOPATH_TMP}" GOBIN="${GOBIN}" go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest || true
GOPATH="${GOPATH_TMP}" GOBIN="${GOBIN}" go install github.com/securego/gosec/v2/cmd/gosec@latest || true
rm -rf "${GOPATH_TMP}"

# ---------- Summary ----------
echo ""
echo "============================================"
echo "  Setup complete!"
echo "============================================"
echo "  Go:      $(go version)"
echo "  Node:    $(node -v)"
echo "  npm:     $(npm -v)"
echo "  Docker:  $(docker --version)"
echo "  Make:    $(make --version | head -1)"
echo "  gcc:     $(gcc --version | head -1)"
echo ""
echo "Next steps:"
echo "  1. Log out and back in (or run: source /etc/profile.d/go.sh)"
echo "  2. cd into the project directory"
echo "  3. make frontend-install   # install frontend deps"
echo "  4. make build-all          # build frontend + backend"
echo "  5. make test               # run tests"
echo "  6. cp config.example.yaml config.yaml  # configure API keys"
