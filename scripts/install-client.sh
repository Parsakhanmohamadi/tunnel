#!/usr/bin/env bash
set -euo pipefail

BIN_NAME="tunnel-client"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/customtunnel"
SERVICE_NAME="tunnel-client.service"

echo "[*] Installing tunnel client..."

sudo mkdir -p "${CONFIG_DIR}"

if [[ -f "./${BIN_NAME}" ]]; then
  echo "[*] Copying binary to ${INSTALL_DIR}/${BIN_NAME}"
  sudo cp "./${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
  sudo chmod +x "${INSTALL_DIR}/${BIN_NAME}"
else
  echo "[!] Binary ./${BIN_NAME} not found. Build it with:"
  echo "    GOOS=linux GOARCH=amd64 go build -o ${BIN_NAME} ./cmd/client"
  exit 1
fi

if [[ -f "../config/tunnel-client.yaml" ]]; then
  echo "[*] Copying default client config to ${CONFIG_DIR}/tunnel-client.yaml"
  sudo cp "../config/tunnel-client.yaml" "${CONFIG_DIR}/tunnel-client.yaml"
fi

echo "[*] Creating systemd service ${SERVICE_NAME}"
sudo tee "/etc/systemd/system/${SERVICE_NAME}" >/dev/null <<EOF
[Unit]
Description=Custom TLS tunnel client
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BIN_NAME} -config ${CONFIG_DIR}/tunnel-client.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

echo "[*] Reloading systemd daemon"
sudo systemctl daemon-reload
echo "[*] Enable and start service"
sudo systemctl enable "${SERVICE_NAME}"
sudo systemctl start "${SERVICE_NAME}"

echo "[*] Done. Check status with: sudo systemctl status ${SERVICE_NAME}"

