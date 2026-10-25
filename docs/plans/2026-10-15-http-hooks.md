# Vendor-CLI hooks without a process per event

## Problem

Every hook a vendor CLI fires ran `crowbar hook <event>`: a fresh exec of the
~52 MB daemon binary (~35-45 ms CPU, the binary mapped each time) that dialled
the daemon and exited. A recorded Claude session with a handful of tool calls is
44 events; a real agent run is hundreds. Endpoint-security agents charge per
process, which is what made Falcon spike while Crowbar was open.

## Design

- **Claude posts its hooks itself.** Claude Code supports `type: "http"` hooks
  (url, headers, timeout; loopback is allowed). The descriptor points all 17
  events at the daemon, so an event costs a request, not a process.
- **Codex cannot.** Codex's hooks are `command`-only (prompt and agent hooks are
  "not supported yet"), and only its terminal surface uses hooks at all (chat
  runs `codex app-server`). Its commands, and Claude's `statusLine`, which is
  also command-only, run `curl` against the same endpoint instead of the relay.
  curl reads the runner token from a `0600` file under the runner's temp dir, so
  it never sits in argv.
- **One endpoint, runner-keyed.** `POST .../chats/runners/:segid/hooks/:provider/:event`
  (repo and project-home mounts) beside the existing `.../runners/:segid/mcp`.
  The body is the provider's payload; `X-Crowbar-Runner-Token` is the per-runner
  token the MCP bridge already uses (`TokenMinter`), verified per request.
- **The response is the answer channel.** A hook that opens a prompt a human can
  decide is held open by the daemon until they do, and the answer is the body.
  The relay used to poll `/hooks/await` and send `/hooks/abandon` on SIGTERM;
  here a CLI that hangs up has been decided at the terminal, so the daemon
  abandons the prompt on disconnect.
- **A second, loopback-only listener** (`core/hookserver`) serves exactly that
  route (everything else 404s at the gate), because the main API is
  unauthenticated and must not be reachable on a TCP port any local process can
  dial. Its address is published to `<home>/run/hooks.url` at every boot and read
  by the spawner. Nothing needs to survive a restart: runners are children of the
  daemon and the token secret is per boot.

## Not changed

`crowbar hook` stays for descriptors that still use `{crowbar_hook}` (user-uploaded
ones); the shipped descriptors no longer do. The live descriptor harness's
recorder now also serves HTTP so `make descriptor-live` still records hooks.

## Verified

Real `claude` 2.1.300 run with the `settings.json` a live daemon rendered for a
runner: its `UserPromptSubmit` hook reached the daemon over loopback and the
prompt appeared in the chat journal; no `crowbar hook` exec occurred. A missing or
wrong token is 401, and any other path on the hook port is 404. A dead
`HTTP(S)_PROXY` does not affect loopback hooks.

## Open

- Claude's `statusLine` and Codex's hooks still exec once per event, but as
  `curl`, a small system binary, not the 52 MB relay.
- Codex was checked against 0.162.1; if it gains an HTTP hook type, the
  `hook_curl` commands in `codex.yaml` can become `http` entries like Claude's.
