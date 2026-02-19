#!/usr/bin/env bash
set -euo pipefail

# Simple one-shot installer similar to paqet-tunnel's deploy script.
# Usage (run on each server separately):
#   curl -fsSL https://raw.githubusercontent.com/Parsakhanmohamadi/tunnel/main/deploy.sh -o deploy.sh
#   chmod +x deploy.sh
#   sudo ./deploy.sh

REPO_URL="https://github.com/Parsakhanmohamadi/tunnel.git"
INSTALL_DIR="/opt/tunnel"
CONFIG_DIR="/etc/customtunnel"

echo "=== Tunnel deploy ==="

if [[ $EUID -ne 0 ]]; then
  echo "[!] Please run this script as root (sudo ./deploy.sh)"
  exit 1
fi

read -rp "Server role? (server/client): " ROLE
ROLE=${ROLE,,}
if [[ "$ROLE" != "server" && "$ROLE" != "client" ]]; then
  echo "[!] Invalid role. Only 'server' or 'client' is allowed."
  exit 1
fi

echo "[*] Installing prerequisites (git, golang)..."
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y
  apt-get install -y git golang
else
  echo "[!] Only apt-based systems (Debian/Ubuntu) are automatically supported."
  echo "    Please install git and golang manually, then re-run this script."
fi

echo "[*] Cloning/updating repo at ${INSTALL_DIR} ..."
if [[ -d "${INSTALL_DIR}/.git" ]]; then
  git -C "${INSTALL_DIR}" pull --ff-only
else
  mkdir -p "$(dirname "${INSTALL_DIR}")"
  git clone "${REPO_URL}" "${INSTALL_DIR}"
fi

cd "${INSTALL_DIR}"

mkdir -p "${CONFIG_DIR}"

if [[ "$ROLE" == "server" ]]; then
  echo "[*] Configuring tunnel server (abroad/server)..."

  read -rp "TLS listen port (e.g. 8443, default 8443): " LISTEN_PORT
  LISTEN_PORT=${LISTEN_PORT:-8443}

  read -rp "WireGuard UDP address on this server (default 127.0.0.1:51820): " WG_REMOTE
  WG_REMOTE=${WG_REMOTE:-127.0.0.1:51820}

  CERT_FILE="${CONFIG_DIR}/server.crt"
  KEY_FILE="${CONFIG_DIR}/server.key"

  if [[ -f "${CERT_FILE}" && -f "${KEY_FILE}" ]]; then
    echo "[*] Existing TLS cert/key found (${CERT_FILE}, ${KEY_FILE}) – reusing them."
  else
    echo "[*] Generating self-signed TLS certificate for testing..."
    openssl req -x509 -newkey rsa:2048 -nodes -keyout "${KEY_FILE}" -out "${CERT_FILE}" -days 365 \
      -subj "/CN=tunnel-server"
  fi

  cat > "${CONFIG_DIR}/tunnel-server.yaml" <<EOF
listen_addr: ":${LISTEN_PORT}"

tls_cert_file: "${CERT_FILE}"
tls_key_file: "${KEY_FILE}"

wireguard_remote: "${WG_REMOTE}"
EOF

  echo "[*] Building server binary..."
  GOOS=linux GOARCH=amd64 go build -o tunnel-server ./cmd/server

  echo "[*] Running server install script..."
  chmod +x scripts/install-server.sh
  ./scripts/install-server.sh

  echo "[*] Tunnel server installed. Status:"
  systemctl status tunnel-server.service --no-pager || true

else
  echo "[*] Configuring tunnel client (Iran/client)..."

  read -rp "Tunnel server address (e.g. your-domain.com:8443): " SERVER_ADDR
  if [[ -z "${SERVER_ADDR}" ]]; then
    echo "[!] server_addr cannot be empty."
    exit 1
  fi

  read -rp "WireGuard UDP address on this server (default 127.0.0.1:51820): " WG_LOCAL
  WG_LOCAL=${WG_LOCAL:-127.0.0.1:51820}

  CA_FILE_DEFAULT="${CONFIG_DIR}/ca.crt"
  read -rp "CA cert path for server (empty to use system CAs, default ${CA_FILE_DEFAULT} if it exists): " CA_FILE
  if [[ -z "${CA_FILE}" && -f "${CA_FILE_DEFAULT}" ]]; then
    CA_FILE="${CA_FILE_DEFAULT}"
  fi

  if [[ -n "${CA_FILE}" && ! -f "${CA_FILE}" ]]; then
    echo "[!] CA file ${CA_FILE} not found; falling back to system CAs."
    CA_FILE=""
  fi

  cat > "${CONFIG_DIR}/tunnel-client.yaml" <<EOF
server_addr: "${SERVER_ADDR}"
ca_cert_file: "${CA_FILE}"
wireguard_local: "${WG_LOCAL}"
EOF

  echo "[*] Building client binary..."
  GOOS=linux GOARCH=amd64 go build -o tunnel-client ./cmd/client

  echo "[*] Running client install script..."
  chmod +x scripts/install-client.sh
  ./scripts/install-client.sh

  echo "[*] Tunnel client installed. Status:"
  systemctl status tunnel-client.service --no-pager || true
fi

echo "=== Done. To view logs:"
if [[ "$ROLE" == "server" ]]; then
  echo "  journalctl -u tunnel-server.service -f"
else
  echo "  journalctl -u tunnel-client.service -f"
fi

