# Local changes to minivpn

This is a vendored copy of [ooni/minivpn](https://github.com/ooni/minivpn) v0.0.7 (GPL-3.0-or-later). It is modified to work with SoftEther-based OpenVPN servers such as the VPN Gate relays.

| File | Change |
|---|---|
| `internal/tlssession/tlshandshake.go` | Peer certificate verification now uses the intermediates the server sends. Previously, leaf certificates issued by a public CA (e.g. Let's Encrypt) failed with "unknown authority". |
| `internal/model/tunnelinfo.go`, `internal/tlssession/controlmsg.go`, `internal/session/manager.go` | Track whether the server pushed a `peer-id`. |
| `internal/datachannel/write.go`, `read.go`, `controller.go`, `internal/model/packet.go` | Use `P_DATA_V1` framing when no `peer-id` was pushed, as the OpenVPN protocol requires. Previously `P_DATA_V2` was always sent, and servers that only speak V1 silently dropped it. |
| `internal/datachannel/service.go` | Received keepalive pings are dropped without printing a hex dump. |
| `pkg/config/vpnoptions.go` | `ReadConfig` parses a config from memory. Android apps cannot create files in `os.TempDir`. |
| `internal/tun/tun.go` | The TUN closes when the workers shut down, so a connection closed by the server surfaces as a Read/Write error instead of a hang. |

The `datachannel` unit tests still assume `P_DATA_V2` framing without a pushed `peer-id`, and fail accordingly.
