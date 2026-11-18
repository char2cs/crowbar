# browseragent

## Owns

The desktop end of the daemon's browser tools: keeping one window attached as the daemon's browser
peer, decoding the daemon's requests into commands for the shell, and matching the shell's answers
to the requests that wait for them.

## Public interface

- `run_peer`: the connection loop of one window's peer link.
- Ports: `PeerConnector` and `PeerStream` (the socket), `TabCommands` (what the shell does).
- `PeerSession`, `AgentCommand`, `AgentOp`, `DomOp`, `TabSummary`: what the shell is asked.
- `Replies`, `Expectation`, `Arming`, `Delivery`: answers matched to waiting requests.
- `Error`.

## Depends on

- `browser`: `AgentAnswer`, the page's answer to an agent operation.
