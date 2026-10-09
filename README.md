# VV Browser

A dedicated browser for the cloud version of DEAD OR ALIVE Xtreme Venus Vacation on DMM GAMES, for Android and PC.

The game is only available from Japan. VV Browser connects to a Japanese [VPN Gate](https://www.vpngate.net/) relay inside the app. Only the hosts that check your region (DMM and the cloud-gaming login) go through it, and the game's video stream connects directly for lower latency. No system-wide VPN is used, so other apps and VPNs on the device are unaffected.

> Accessing DMM GAMES from outside its supported regions may violate DMM's terms of service. Use at your own risk.

## Status

- `core/`: the network core, a Go library. It also runs as a standalone local proxy that any browser can use.
- `android/`: the Android app (Android 13 or newer), a full-screen WebView routed through the core.
- `desktop/`: the Windows app, a Tauri window that runs the core as a sidecar process.

Both apps request the stream's high quality tier (1280x720, 6-8 Mbps) rather than the default 2 Mbps.

## Android app

### Requirements

- Android SDK with an NDK installed, and `ANDROID_HOME` set
- JDK 17 or newer (Android Studio's bundled JBR works)
- Go 1.26 or newer

### Build

```
sh scripts/build-core.sh
cd android
./gradlew assembleDebug
```

The APK is written to `android/app/build/outputs/apk/debug/app-debug.apk`.

Sign in with your DMM email address and password. Google sign-in does not work inside an embedded WebView.

## Desktop app (Windows)

### Requirements

- Rust (stable) with the MSVC toolchain
- Node.js 20 or newer
- Go 1.26 or newer
- Microsoft Edge WebView2 Runtime (included with Windows 11)

### Build

```
sh scripts/build-desktop-core.sh
cd desktop
npm install
npm run build
```

The installer is written to `desktop/src-tauri/target/release/bundle/nsis/`. For a development build, run `npm run dev` instead of `npm run build`.

## Network core

### Requirements

- Go 1.26 or newer

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
