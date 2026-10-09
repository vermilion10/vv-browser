# VV Browser

A dedicated browser for the cloud version of DEAD OR ALIVE Xtreme Venus Vacation on DMM GAMES, for Android and PC.

The game is only available from Japan. VV Browser connects to a Japanese [VPN Gate](https://www.vpngate.net/) relay inside the app. Only the hosts that check your region (DMM and the cloud-gaming login) go through it, and the game's video stream connects directly for lower latency. No system-wide VPN is used, so other apps and VPNs on the device are unaffected.

> Accessing DMM GAMES from outside its supported regions may violate DMM's terms of service. Use at your own risk.
>
> VV Browser is an unofficial project. It is not affiliated with or endorsed by DMM, Koei Tecmo, Ubitus or VPN Gate. DEAD OR ALIVE Xtreme Venus Vacation is a trademark of its respective owner.

## Status

- `core/`: the network core, a Go library. It also runs as a standalone local proxy that any browser can use.
- `android/`: the Android app (Android 13 or newer), a full-screen WebView routed through the core.
- `desktop/`: the Windows app, a Tauri window that runs the core as a sidecar process.

## Features

- **No system VPN.** The Japanese connection lives inside the app, and only the hosts that check your region use it.
- **Stable relay.** The app remembers the relay that worked and reuses it on later launches, so DMM keeps seeing the same Japanese IP. Frequent IP changes can trigger DMM's suspicious-login checks.
- **Switch relay.** If the relay becomes slow or unreliable, switch to the next best one from the menu. This changes the IP DMM sees, so use it only when needed.
- **Stream quality.** Choose High (1280×720, 6–8 Mbps, the default), Medium (1280×720, 2 Mbps) or Low (960×540, 2 Mbps). The DMM page on its own always uses Medium.
- **Full-screen game.** DMM's header and footer are hidden so the player fills the window. You can turn this off.

On Android, press **Back** for the in-game menu (Resume, Reload game, Switch relay, Stream quality, DMM header and footer, Exit). On Windows, use the **Game** menu; F5 reloads the game.

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

For a release build (phones and tablets only, arm64), run `./gradlew assembleRelease`. To sign it, add these properties to the `gradle.properties` file in your Gradle user home (`~/.gradle`, or `GRADLE_USER_HOME` if set). Without them, the release APK is left unsigned.

```
VV_RELEASE_STORE_FILE=/path/to/release.jks
VV_RELEASE_STORE_PASSWORD=...
VV_RELEASE_KEY_ALIAS=...
VV_RELEASE_KEY_PASSWORD=...
```

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

## Acknowledgements

- [minivpn](https://github.com/ooni/minivpn) by OONI: the userspace OpenVPN client the core is built on
- [wireguard-go](https://git.zx2c4.com/wireguard-go/) netstack, based on [gVisor](https://gvisor.dev/): the userspace TCP/IP stack
- [VPN Gate](https://www.vpngate.net/), the academic public relay service run by the University of Tsukuba
- [Tauri](https://tauri.app/) for the desktop app

## License

GPL-3.0-or-later; see `LICENSE`. The bundled OpenVPN implementation is a modified copy of [minivpn](https://github.com/ooni/minivpn); see `core/third_party/minivpn/VV_PATCHES.md`.
