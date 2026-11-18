# protocol

## Owns

The wire vocabulary shared by the shell, its panes and the daemon transport: the identifiers every
crate uses, the `sdk@1` control frames a pane and the shell exchange (with their JSON codec and
size caps), and the port through which the shell reaches a daemon.

## Public interface

- Identifiers: `HostId`, `PaneId`, `SpaceId`, `CommandId`, `ExtensionId`, `PaneTypeId`, `PaneKind`, `PaneOrigin`.
- Frames: `PaneFrame` and `ShellFrame` with `decode_pane_frame`, `decode_shell_frame`, `encode` and
  `MAX_FRAME_BYTES`; the bounded values they carry (`PaneState`, `PaneTitle`, `Params`, `DisplayText`,
  `ExternalUrl`, `FsPath`, `ExtensionStatus`, `StatusKind`, `SettingKey`, `SettingValues`,
  `ThemeTokens`, `KeyBinding`, `KeyChord`); request and response frames (`RequestId`, `MethodName`,
  `EventName`, `RpcResponse`, `RpcOutcome`, `RpcError`, `ErrorCode`); `SdkVersion` and `Lifecycle`.
- The daemon port: `DaemonTransport` and `Responder`, `DaemonRequest`, `DaemonResponse`,
  `HttpMethod`, `RequestTarget`, `RelayEndpoint`, `RelayScope`.
- `Error` and `Result`.

## Depends on

No workspace crate (serde, serde_json, thiserror).
