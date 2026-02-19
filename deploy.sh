#!/usr/bin/env bash
set -euo pipefail

# Simple one-shot installer similar to paqet-tunnel's deploy script.
# Usage (روی هر سرور جداگانه):
#   curl -fsSL https://raw.githubusercontent.com/Parsakhanmohamadi/tunnel/main/deploy.sh -o deploy.sh
#   chmod +x deploy.sh
#   sudo ./deploy.sh

REPO_URL="https://github.com/Parsakhanmohamadi/tunnel.git"
INSTALL_DIR="/opt/tunnel"
CONFIG_DIR="/etc/customtunnel"

echo "=== Tunnel deploy ==="

if [[ $EUID -ne 0 ]]; then
  echo "[!] لطفاً اسکریپت را با sudo اجرا کنید (sudo ./deploy.sh)"
  exit 1
fi

read -rp "نقش این سرور چیست؟ (server/client): " ROLE
ROLE=${ROLE,,}
if [[ "$ROLE" != "server" && "$ROLE" != "client" ]]; then
  echo "[!] نقش نامعتبر. فقط server یا client مجاز است."
  exit 1
fi

echo "[*] نصب پیش‌نیازها (git, golang)..."
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y
  apt-get install -y git golang
else
  echo "[!] فقط سیستم‌های مبتنی بر apt (Debian/Ubuntu) به‌صورت خودکار پشتیبانی شده‌اند."
  echo "    خودت git و golang را نصب کن و دوباره اسکریپت را اجرا کن."
fi

echo "[*] کلون/آپدیت ریپو در ${INSTALL_DIR} ..."
if [[ -d "${INSTALL_DIR}/.git" ]]; then
  git -C "${INSTALL_DIR}" pull --ff-only
else
  mkdir -p "$(dirname "${INSTALL_DIR}")"
  git clone "${REPO_URL}" "${INSTALL_DIR}"
fi

cd "${INSTALL_DIR}"

mkdir -p "${CONFIG_DIR}"

if [[ "$ROLE" == "server" ]]; then
  echo "[*] تنظیم سرور تونل (abroad/server)..."

  read -rp "پورت شنود TLS (مثلاً 8443، پیش‌فرض 8443): " LISTEN_PORT
  LISTEN_PORT=${LISTEN_PORT:-8443}

  read -rp "آدرس UDP WireGuard روی این سرور (پیش‌فرض 127.0.0.1:51820): " WG_REMOTE
  WG_REMOTE=${WG_REMOTE:-127.0.0.1:51820}

  CERT_FILE="${CONFIG_DIR}/server.crt"
  KEY_FILE="${CONFIG_DIR}/server.key"

  if [[ -f "${CERT_FILE}" && -f "${KEY_FILE}" ]]; then
    echo "[*] گواهی TLS موجود پیدا شد (${CERT_FILE}, ${KEY_FILE}) – از همان استفاده می‌کنیم."
  else
    echo "[*] ساخت گواهی self-signed برای تست..."
    openssl req -x509 -newkey rsa:2048 -nodes -keyout "${KEY_FILE}" -out "${CERT_FILE}" -days 365 \
      -subj "/CN=tunnel-server"
  fi

  cat > "${CONFIG_DIR}/tunnel-server.yaml" <<EOF
listen_addr: ":${LISTEN_PORT}"

tls_cert_file: "${CERT_FILE}"
tls_key_file: "${KEY_FILE}"

wireguard_remote: "${WG_REMOTE}"
EOF

  echo "[*] build باینری سرور..."
  GOOS=linux GOARCH=amd64 go build -o tunnel-server ./cmd/server

  echo "[*] اجرای نصب server..."
  chmod +x scripts/install-server.sh
  ./scripts/install-server.sh

  echo "[*] سرور تونل نصب شد. وضعیت:"
  systemctl status tunnel-server.service --no-pager || true

else
  echo "[*] تنظیم کلاینت تونل (Iran/client)..."

  read -rp "آدرس سرور تونل (مثلاً your-domain.com:8443): " SERVER_ADDR
  if [[ -z "${SERVER_ADDR}" ]]; then
    echo "[!] server_addr نمی‌تواند خالی باشد."
    exit 1
  fi

  read -rp "آدرس UDP WireGuard روی این سرور (پیش‌فرض 127.0.0.1:51820): " WG_LOCAL
  WG_LOCAL=${WG_LOCAL:-127.0.0.1:51820}

  CA_FILE_DEFAULT="${CONFIG_DIR}/ca.crt"
  read -rp "مسیر CA cert برای سرور (خالی برای استفاده از CA سیستم، پیش‌فرض ${CA_FILE_DEFAULT} اگر وجود داشته باشد): " CA_FILE
  if [[ -z "${CA_FILE}" && -f "${CA_FILE_DEFAULT}" ]]; then
    CA_FILE="${CA_FILE_DEFAULT}"
  fi

  if [[ -n "${CA_FILE}" && ! -f "${CA_FILE}" ]]; then
    echo "[!] فایل CA ${CA_FILE} پیدا نشد؛ از CA سیستم استفاده می‌شود."
    CA_FILE=""
  fi

  cat > "${CONFIG_DIR}/tunnel-client.yaml" <<EOF
server_addr: "${SERVER_ADDR}"
ca_cert_file: "${CA_FILE}"
wireguard_local: "${WG_LOCAL}"
EOF

  echo "[*] build باینری کلاینت..."
  GOOS=linux GOARCH=amd64 go build -o tunnel-client ./cmd/client

  echo "[*] اجرای نصب client..."
  chmod +x scripts/install-client.sh
  ./scripts/install-client.sh

  echo "[*] کلاینت تونل نصب شد. وضعیت:"
  systemctl status tunnel-client.service --no-pager || true
fi

echo "=== تمام شد. برای مشاهده لاگ‌ها:"
if [[ "$ROLE" == "server" ]]; then
  echo "  journalctl -u tunnel-server.service -f"
else
  echo "  journalctl -u tunnel-client.service -f"
fi

