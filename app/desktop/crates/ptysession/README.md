# ptysession

## Owns

The life of one terminal session, from its creation on the daemon to the end of its stream: the
REST calls that create and kill it, and the WebSocket pumped over the shell's loopback relay,
including reconnects and recovery.

## Public interface

- `TerminalSessions`: creates or attaches a chat's terminal session and runs its stream.
- `Ends`: the terminal's ends of a running session.
- `SessionBase`: the daemon paths of one chat's terminal sessions.
- `Error`, `Result`.

## Depends on

- `protocol`: the daemon transport port and relay scopes.
- `terminal`: the terminal model fed by the stream.
- `daemonapi`: the reply envelope.
- External: tokio, tokio-tungstenite, futures-util, async-channel.
