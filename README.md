# VV Browser

A dedicated browser for the cloud version of DEAD OR ALIVE Xtreme Venus Vacation on DMM GAMES, for Android and PC.

The game is only available from Japan. VV Browser connects to a Japanese [VPN Gate](https://www.vpngate.net/) relay inside the app. Only the hosts that check your region (DMM and the cloud-gaming login) go through it, and the game's video stream connects directly for lower latency. No system-wide VPN is used, so other apps and VPNs on the device are unaffected.

> Accessing DMM GAMES from outside its supported regions may violate DMM's terms of service. Use at your own risk.

## Status

Only the network core (`core/`) exists so far. It runs as a standalone local proxy that any browser can use.

## Network core

### Requirements

- Go 1.25 or newer

### Build

```
cd core
go build -o ../bin/vvcore.exe ./cmd/vvcore
```

### Usage

Check that the tunnel works. This prints the IP and country the tunnel exits from:

```
vvcore -check
```

Run the proxy:

```
vvcore -listen 127.0.0.1:8899
```

Then point a browser at it, for example Chrome:

```
chrome.exe --user-data-dir=%TEMP%\vvbrowser --proxy-server=http://127.0.0.1:8899
```

| Flag | Default | Description |
|---|---|---|
| `-listen` | `127.0.0.1:8899` | Address of the local HTTP proxy |
| `-country` | `JP` | VPN Gate country to choose relays from |
| `-ovpn` | | Use this OpenVPN config file instead of the VPN Gate server list |
| `-mtu` | `1400` | MTU of the in-app tunnel interface |
| `-check` | | Connect, print the exit IP information and quit |
| `-v` | | Verbose OpenVPN logging |

### Routing

| Hosts | Route |
|---|---|
| `*.dmm.com`, `*.dmm.co.jp`, `*.dmmapis.com`, `dcgp-game.ugamenow.com`, `ipinfo.io` | Through the Japanese relay |
| `gc-*.ugamenow.com` (game stream) and everything else | Direct |

## License

GPL-3.0-or-later. The bundled OpenVPN implementation is a modified copy of [minivpn](https://github.com/ooni/minivpn); see `core/third_party/minivpn/VV_PATCHES.md`.
