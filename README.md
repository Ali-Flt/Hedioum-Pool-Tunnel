# Hedioum Dynamic Pool Tunnel (Chaos Mesh)

Hedioum Pool Tunnel is a high-performance, enterprise-grade connection multiplexer designed to bypass strict Deep Packet Inspection (DPI) and thwart TCP Meltdown under heavy load. It operates as a Custom SDN Overlay, wrapping encrypted VLESS/Trojan traffic into highly obfuscated, dynamically scaling SSH-mimicked connection pools.

## 🌟 Key Features

- **Chaos Mesh Dynamic Balancing:** Replaces traditional Round-Robin with a smart "Least Loaded" distribution algorithm. The system actively monitors real-time bandwidth (Mbps) and scales physical connections up or down based on actual traffic volume, not just stream count.
- **DPI Evasion (Fluctuating Caps):** Implements dynamic bandwidth jitter. Each physical connection operates under a randomized, fluctuating bandwidth limit (e.g., 8 Mbps ± 2 Mbps) to break static patterns, making the tunnel indistinguishable from organic, noisy internet traffic.
- **Zero-Downtime Connection Draining:** During scale-down events, idle physical connections are placed in a `Draining` state. They wait for active logical streams (like open socket connections) to finish naturally before closing, ensuring zero lag or disconnections for end-users.
- **Enterprise Lifecycle Management:** Features an interactive TUI dashboard equipped with a Blue-Green Self-Updater (with automatic rollback on failure) and a Clean Uninstaller that purges all traces without leaving orphaned files.
- **Zero Double-Encryption Overhead:** Pipes natively encrypted X-UI traffic without re-encrypting with AES, keeping CPU usage near zero on low-end servers.
- **Protocol Mimicking:** Accurately simulates the SSH-2.0-OpenSSH handshake and binary framing, coupled with cryptographically secure random noise padding to obscure metadata.

## 🏗 Architecture Topology

1. **X-UI (Iran):** Authenticates the user, splits domestic traffic, and forwards international traffic to the local SOCKS5 Bridge.
2. **Hedioum Hub (Iran):** Receives the SOCKS5 payload, evaluates pool health, and multiplexes the stream (via HashiCorp Yamux) over an SSH-mimicked physical connection pool using the Chaos Mesh algorithm.
3. **Hedioum Egress (Foreign):** Validates the SSH handshake token, enforces SSRF protections, extracts target metadata, and dials the open internet directly over forced IPv4 sockets.

## 🚀 Installation & Seamless Updates

You can deploy the Hedioum daemon on any Ubuntu/Debian server using our 1-click installation script. The script automatically fetches the latest compiled release from GitHub and preserves your configuration across updates.

**Installation Order:** You MUST install the Foreign Node first to generate the Authentication Token required by the Iran Node.

### Step 1: Deploy Foreign Node (Egress)
Run the following command on your foreign VPS:

    bash <(curl -s https://raw.githubusercontent.com/Ali-Flt/Hedioum-Pool-Tunnel/main/install.sh)

Follow the interactive wizard. Copy the generated Auth Token.

### Step 2: Deploy Iran Node (Hub)
Run the same command on your Iran VPS:

    bash <(curl -s https://raw.githubusercontent.com/Ali-Flt/Hedioum-Pool-Tunnel/main/install.sh)

Select "Iran Node" and add your Foreign Node. You will be prompted to define your DPI evasion parameters (Bandwidth Limits & Jitter) during setup.

### Unattended / Automated Installation
For scripted deployments (Ansible, cloud-init, image baking), skip the interactive wizard by passing a config file.
The file uses the exact same schema the daemon persists to `/etc/hedioum/hedioum.json`.
Ready-to-edit templates live in [`config/examples/`](config/examples/).

    # Validate your config first (no root, no changes made):
    bash install.sh --config ./hedioum.json --validate-only

    # Provision headlessly (installs binary + service, then starts the daemon):
    sudo bash install.sh --config ./hedioum.json
    # or: sudo HEDIOUM_CONFIG=./hedioum.json bash install.sh

Notes:
- Generate the shared auth token once with `openssl rand -hex 16` and use it in both the foreign and Iran configs.
- **Foreign nodes bind port 22**, so free it before an unattended install (relocate SSH, e.g. to 2022). The interactive wizard offers to do this move for you; the unattended path does not touch your SSH config.

## ⚙️ Management Dashboard

To manage servers, view live connection status, or perform lifecycle operations, run the interactive dashboard from your terminal at any time:

    hedioum-tunnel

**Dashboard Capabilities:**
- View active egress pools, target IPs, and live DPI Evasion dynamics (Limits & Jitter).
- Monitor real-time daemon logs for Scale-Up/Down events and bandwidth usage.
- Add or Remove foreign egress nodes dynamically.
- Perform a safe Self-Update (fetches latest GitHub release).
- Completely Uninstall and purge the daemon.

## 🛠 Building from Source

If you wish to compile the static binary manually:

    git clone https://github.com/Ali-Flt/Hedioum-Pool-Tunnel.git
    cd Hedioum-Pool-Tunnel
    make build-linux

## ☕ Support & Donate

If you found this project helpful for maintaining a free and open internet, and you want to support further development, consider buying the team a coffee!

**USDT (Tether) Donation Addresses:**
- **TRC20 (Tron):** TRhwZFoHRZ9oux4emFXTj63aib9nuC2J2J
- **BEP20 (BSC):** 0x051e31cb70076854C0b62F816d5a89D3def4A22E
- **ERC20 (Ethereum):** 0x051e31cb70076854C0b62F816d5a89D3def4A22E
- **TON (The Open Network):** UQCqq0wYNDVhq9AXAZ5vOQ2ZgMmP6O0UTgvU1YhNeIpkUp1s

Thank you for your support!