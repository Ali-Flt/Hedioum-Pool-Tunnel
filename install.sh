#!/bin/bash

# ==========================================================
# Hedioum Dynamic Pool Tunnel - 1-Click Installer & Updater
# ==========================================================
#
# Interactive (default): bash install.sh
# Unattended setup:       bash install.sh --config /path/to/hedioum.json
#                         HEDIOUM_CONFIG=/path/to/hedioum.json bash install.sh
# Validate only (no root):bash install.sh --config /path/to/hedioum.json --validate-only
#
# Config templates: config/examples/foreign.json, config/examples/iran.json

CONFIG_FILE="${HEDIOUM_CONFIG:-}"
VALIDATE_ONLY=0

print_usage() {
  cat <<'USAGE'
Usage: install.sh [--config FILE] [--validate-only]

  --config, -c FILE   Provision non-interactively from a hedioum.json config,
                      skipping the interactive setup wizard.
  --validate-only     Validate the --config file and exit (no root, no install).
  -h, --help          Show this help.

Environment:
  HEDIOUM_CONFIG      Alternative to --config.

Templates: config/examples/foreign.json, config/examples/iran.json
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --config|-c) CONFIG_FILE="$2"; shift 2 ;;
    --config=*)  CONFIG_FILE="${1#*=}"; shift ;;
    --validate-only) VALIDATE_ONLY=1; shift ;;
    -h|--help) print_usage; exit 0 ;;
    *) echo "[x] Unknown option: $1"; print_usage; exit 1 ;;
  esac
done

# validate_config FILE: fails fast with a clear message on a bad config.
# The daemon does full validation on load; this just catches the common footguns
# before we touch the system (bad JSON, missing role, foreign node without a token).
validate_config() {
  local f="$1"
  if [ ! -f "$f" ]; then echo "[x] Config file not found: $f"; return 1; fi
  if [ ! -s "$f" ]; then echo "[x] Config file is empty: $f"; return 1; fi

  # Real JSON syntax check when python3 is available (optional, no hard dep).
  if command -v python3 >/dev/null 2>&1; then
    if ! python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$f" 2>/dev/null; then
      echo "[x] Config is not valid JSON: $f"; return 1
    fi
  fi

  local role
  role=$(grep -oE '"role"[[:space:]]*:[[:space:]]*"(iran|foreign)"' "$f" | grep -oE '(iran|foreign)' | head -1)
  if [ -z "$role" ]; then
    echo "[x] Config must set \"role\" to \"iran\" or \"foreign\"."
    return 1
  fi

  # A foreign (egress) node with no token = open, unauthenticated egress. Block it.
  if [ "$role" = "foreign" ] && ! grep -Eq '"auth_token"[[:space:]]*:[[:space:]]*"[^"]+"' "$f"; then
    echo "[x] Foreign node config requires a non-empty \"auth_token\"."
    echo "    Generate one with: openssl rand -hex 16"
    return 1
  fi

  echo "[✓] Config looks valid (role: $role)."
  return 0
}

if [ "$VALIDATE_ONLY" -eq 1 ]; then
  if [ -z "$CONFIG_FILE" ]; then echo "[x] --validate-only requires --config FILE"; exit 1; fi
  validate_config "$CONFIG_FILE"; exit $?
fi

# Fail fast on a bad config before requiring root or downloading anything.
if [ -n "$CONFIG_FILE" ]; then
  validate_config "$CONFIG_FILE" || exit 1
fi

if [ "$EUID" -ne 0 ]; then
  echo "[x] CRITICAL: Please run the installer as root (e.g., sudo bash install.sh)"
  exit 1
fi

echo "=================================================="
echo "  Deploying Hedioum Stealth Mesh Daemon..."
echo "=================================================="

mkdir -p /etc/hedioum
mkdir -p /usr/local/bin

# --- Unattended provisioning: drop the supplied config into place ---
# Done before the wizard check below so the binary boots straight into daemon mode.
if [ -n "$CONFIG_FILE" ]; then
    echo "[*] Applying provided configuration (unattended setup)..."
    cp "$CONFIG_FILE" /etc/hedioum/hedioum.json
    chmod 600 /etc/hedioum/hedioum.json
fi

# --- Stop service and unlink binary to prevent 'Text file busy' error ---
if systemctl is-active --quiet hedioum.service; then
    echo "[*] Stopping existing daemon to apply update..."
    systemctl stop hedioum.service > /dev/null 2>&1
fi
rm -f /usr/local/bin/hedioum-tunnel

# --- Architecture Detection ---
OS_ARCH=$(uname -m)
TARGET_ASSET="hedioum-tunnel"

if [ "$OS_ARCH" = "aarch64" ] || [ "$OS_ARCH" = "arm64" ]; then
    TARGET_ASSET="hedioum-tunnel-arm64"
    echo "[*] Detected ARM64 architecture."
else
    echo "[*] Detected AMD64/x86_64 architecture."
fi

# --- Dynamic Release Downloader (GitHub API) ---
echo "[*] Fetching the latest release from GitHub..."

# Match exactly target asset using double quotes to avoid partial matches
LATEST_URL=$(curl -s https://api.github.com/repos/Ali-Flt/Hedioum-Pool-Tunnel/releases/latest | grep "browser_download_url" | grep "$TARGET_ASSET\"" | cut -d '"' -f 4)

# Fallback URLs in case GitHub API is rate-limited or blocked
if [ -z "$LATEST_URL" ]; then
    echo "[-] GitHub API rate-limited or blocked. Falling back to static release link..."
    FALLBACK_VERSION="v0.3.2"
    LATEST_URL="https://github.com/Ali-Flt/Hedioum-Pool-Tunnel/releases/download/${FALLBACK_VERSION}/${TARGET_ASSET}"
fi

URL_PROXY="https://ghp.ci/$LATEST_URL"

if curl -f -L -s -o /usr/local/bin/hedioum-tunnel "$LATEST_URL"; then
    echo "[✓] Binary downloaded successfully (Direct Release)."
elif curl -f -L -s -o /usr/local/bin/hedioum-tunnel "$URL_PROXY"; then
    echo "[✓] Binary downloaded successfully (Proxy Fallback for Iran Hub)."
else
    echo "[x] ERROR: Failed to download the binary. Network is severely restricted."
    echo "    Try running the installer again later, or use a VPN."
    exit 1
fi

chmod +x /usr/local/bin/hedioum-tunnel

# --- Configuring Systemd background service ---
echo "[*] Configuring Systemd background service..."
cat << 'EOF' > /etc/systemd/system/hedioum.service
[Unit]
Description=Hedioum Dynamic Pool Tunnel Daemon
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/etc/hedioum
ExecStart=/usr/local/bin/hedioum-tunnel
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable hedioum.service > /dev/null 2>&1

echo "=================================================="
if [ -n "$CONFIG_FILE" ]; then
    echo "[✓] Unattended configuration applied from: $CONFIG_FILE"
    systemctl restart hedioum.service
    echo "[✓] Hedioum Daemon provisioned and running headlessly."
elif [ ! -f "/etc/hedioum/hedioum.json" ]; then
    echo -e "[!] Fresh installation detected. Launching Initial Setup Wizard..."
    sleep 2
    cd /etc/hedioum && hedioum-tunnel
    echo -e "\n[✓] Setup complete! Hedioum Daemon is now running in the background."
else
    echo "[✓] Existing configuration found. Applying seamless update..."
    systemctl restart hedioum.service
    echo "[✓] Hedioum Daemon updated and restarted gracefully."
fi

echo "=================================================="
echo " [Ops] Management Dashboard Command:"
echo " Simply type 'hedioum-tunnel' anywhere in your terminal."
echo "=================================================="
