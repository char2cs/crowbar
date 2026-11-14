# Loopback relay: security review

Status: DRAFT for owner review, updated after the round 1 review fixes. Covers `app/crates/transport` (relay, token book, access scopes, remote hosts). Findings that need an owner decision are in section 6; deliberate differences from the Tauri originals are in section 7.

## 1. What is being protected

The shell serves panes (web views) that talk to a Crowbar daemon. A pane's `WebSocket` cannot dial a unix socket, so the shell runs a relay on `127.0.0.1:<random port>`. The daemon can read and write the user's repositories, run terminals and drive agents, so a stream into it is equivalent to a shell. Assets:

- the daemon (local socket, or a paired remote host);
- the per-host bearer token and TLS pin of a remote host (kept in the OS keychain, never given to a pane);
- the relay tokens issued to panes.

## 2. Threat model

| Attacker | Capability | Goal |
|---|---|---|
| Other local process (same user) | can connect to any loopback port, can read `/proc` metadata and process lists | open a stream into the daemon through the relay |
| Other local user | same, different uid | same |
| Malicious web page in the user's browser | can open `ws://127.0.0.1:<port>` from a page, sets its real `Origin` | reach the daemon from a web page |
| DNS rebinding | a page on `evil.example` re-resolves to `127.0.0.1`, same-origin to itself, `Host: evil.example:<port>` | read or write through the relay without CORS being involved |
| Compromised or hostile extension pane | runs JS in its own origin inside the shell | reach core routes, or another extension's routes and blobs |
| Network attacker on a remote host path | can intercept or redirect the connection | read or replace the bearer token, impersonate the host |

Out of scope: an attacker with the user's own privileges who can ptrace the shell or read its memory (it could equally read the keychain or talk to the unix socket directly), and a compromised daemon.

## 3. Design

- **Bind.** `127.0.0.1` only, random port, one listener per shell process.
- **Token.** 256 bits from the OS random source, base64url, issued per pane and host (`RelayScope`: host, pane, `PaneOrigin`; an extension origin always carries its extension id and a core origin never does, so a contradictory scope cannot be built). Held hashed (SHA-256) in the token book, compared by lookup of the hash. Default lifetime 12 hours. Issuing again for the same pane and host replaces the old token. A pane's destruction or a host's removal revokes its tokens and closes its open streams. Revocation cannot be lost: the relay subscribes to host removals before its tasks start; a receiver that lags revokes conservatively every host the registry no longer lists; admitting a stream (check the token, register the stream) and revoking (drop the tokens, cancel the streams) each hold the token book across both steps, so a stream is either refused or cancelled, never left open by a race.
- **Delivery.** The token travels in the WebSocket subprotocol list: `crowbar.relay.v1` and `crowbar.token.<token>`. It is never in the URL, so it is not in logs, history, referrers or the process list. The relay answers with the fixed `crowbar.relay.v1` only and never echoes the token.
- **Handshake checks, in order.** Before any daemon is dialled: `Host` must equal `127.0.0.1:<port>` exactly; exactly one `Origin` header; subprotocol `crowbar.relay.v1` offered; a token present and live; the token's scope accepts that `Origin` (exact string match against the shell's webview origins, never a prefix, `null` and empty rejected); the request path is inside the scope's access rule; a free slot exists. A client over the stream limits is refused `503` before the upgrade. Each failure is a bare HTTP status with no body.
- **Access scopes.** Core tokens reach `/v0/...`. Extension tokens reach only `/v0/ext/<own id>/...`, which holds the extension's calls and its own blobs. Paths are normalised strictly: percent-encoded slashes or dots, `..`, doubled slashes, backslashes, control characters and fragments are refused, so a path cannot mean one thing to the relay and another to the daemon. The same rule applies to HTTP over `crowbar://`. The transport stamps `x-crowbar-extension` on extension requests, HTTP and WebSocket upstream alike, and strips any copy a pane sends, so the daemon can enforce the rule independently. A request target is validated as one absolute path and query (no authority, no `//`, no backslash or fragment) and the dialled host comes from the typed host id only, so a request cannot change which host it reaches. Hop-by-hop headers, `Host`, and pane-controlled framing headers are dropped from pane requests and from daemon answers.
- **Bridge.** Text stays text and binary stays binary. Messages over 64 MiB are refused (close 1009). Protocol errors close 1002 and a peer that stops reading closes 1011. Close codes that may not appear on the wire are replaced. Every write has a deadline and the final close of each side is bounded by a grace period and run on both sides at once, so a client that never reads cannot hold a stream, or the daemon's side of it, open. Opening the upstream is raced against revocation and a timeout. Pings are answered by the library and not forwarded. Idle timeouts are configurable per direction, and a pane may ask for its own upstream idle limit with a third subprotocol `crowbar.idle.<seconds>` (clamped to 5 s to 1 h, this stream only).
- **Limits.** 256 open streams, 32 per pane. Connections still in their handshake are counted separately (64) and release their slot the moment the handshake ends, so an unauthenticated client cannot take a stream's place; overflow is answered `503`. The handshake deadline is 3 s. Accept errors back off rather than spin. Over HTTP, at most 32 requests per host are in flight and the rest wait within the proxy timeout.
- **Remote hosts.** TLS 1.3 only. The certificate is checked by SHA-256 digest of the leaf, pinned when the host is paired (trust on first use, confirmed by the user against the fingerprint the host shows). A changed certificate is reported as `host-identity-changed` and the bearer token is never sent to it. Candidate addresses are dialled with a 250 ms stagger so one dead address costs 250 ms, not the dial budget. The bearer token goes only over the pinned channel. The one unpinned dial reads the fingerprint and sends nothing.
- **Proxy answers.** The transport marks its own answers (`x-crowbar-proxy`) and strips a daemon-supplied copy, so a daemon cannot make a pane believe it was the shell speaking. Markers also name `no-credential` and `unknown-host`.
- **Secrets.** Bearer tokens are held zeroizing, `RelayEndpoint`, `Credential` and the paired-host authorization print redacted in `Debug`, and errors never carry a token.

## 4. Mitigations by threat

**Other local process.** Loopback is reachable by every local process, so the port is not a secret and not a defence. The token is. Without it the relay refuses at the handshake with 401. A same-user process that can read the shell's memory is out of scope. A process that scans ports learns only that something answers. Tokens are 256 bits and are not enumerable. Failed handshakes cost the attacker a handshake slot for at most 3 s and are bounded by their own limit, which never touches the stream limits. Residual: an attacker that sniffs loopback traffic as the same user (for example with `CAP_NET_RAW` or a debugger) sees the subprotocol header. The relay uses no TLS on loopback because a self-signed certificate on a random port cannot be pinned by a pane.

**Other local user.** Same as above. On a multi-user machine, loopback ports are visible to other users, so the token requirement is what keeps them out. The daemon's unix socket is never exposed by the relay except through a valid token.

**Malicious web page.** A browser sets `Origin` to the page's real origin and a page cannot change it. The relay requires a single `Origin` header that exactly equals one of the shell's webview origins. A page also does not hold a token. Two independent checks hold.

**DNS rebinding.** The rebound page sends `Host: evil.example:<port>`. The relay requires `Host: 127.0.0.1:<port>`, so the dial is refused before the token or Origin is looked at. The `Origin` of that page is `http://evil.example:<port>` and is refused as well.

**Token leakage.**
- Not in URLs, so not in history, logs or referrers.
- Stored hashed, so a memory dump of the book does not give out usable tokens. (A dump of the pane's own JS heap does; that pane already holds the token by design.)
- Bound to one pane, one host, one kind. A token leaked from pane A cannot be used with pane B's origin unless the origins coincide.
- Bound to an origin: even a leaked token is useless from any other origin because the Origin check uses the scope the token was issued for.
- Replaced on re-issue, revoked on pane destroy and host removal, expired after the TTL.
- Never written to the host registry, logs (`tracing` never records it) or the keychain.

**Origin spoofing.** A non-browser client can set any `Origin`. Origin is therefore a defence against browsers only; the token is the defence against everything else. A local process that obtained a token and sets the right `Origin` is accepted. It could equally use the pane. Section 5 shows how the daemon-issued scheme narrows this.

**Extension pane.** Its token names its extension. The relay refuses any path outside `/v0/ext/<id>/`. The HTTP proxy applies the same rule and stamps `x-crowbar-extension`. The daemon must enforce the same rule independently (agreed in the extension host protocol, section 8). The relay and the HTTP path share the same `access_scope::check` so they cannot drift. Residual: the `/v0/ext/` prefix is an assumption about the extensions branch's route layout and must be confirmed there.

**Remote hosts.** Pin mismatch fails closed. The token never leaves the keychain for a pane. Pairing records the host list first and the credential second. If either write fails the host list is put back as it was and the token is revoked on the host before the error is returned, so no valid token is left unrecorded. `remove_host` drops the host from the list, tells every relay to end its streams, then deletes the stored credential; a credential that cannot be deleted is reported (`CredentialRemains`) and the host is already unreachable, so calling again retries. The hosts file is written atomically with owner-only permissions and fsynced; duplicate ids are rejected on load.

**Browser tabs.** A browser tab is not a pane and must never reach the daemon through this crate. The rule lives in the shell, not here: `pane_host`'s origin table is default-deny for any webview that is not registered, and a browser tab has no `PaneOrigin`, so it cannot construct a `DaemonRequest` or a `RelayScope`. The relay's exact-origin policy refuses a tab's `Origin` independently. The Tauri original denied browser tabs inside the proxy by checking a webview label; with typed origins that check is structural.

## 5. Tests that hold these properties

`app/crates/transport/tests/relay.rs` and the unit tests beside the code cover, against real sockets: wrong, missing, expired, replaced and revoked tokens; wrong, missing, duplicate, `null` and prefix-matching `Origin`; wrong `Host` (rebinding); missing subprotocol; extension token reaching a core path or another extension and the extension header on the upstream upgrade; revocation racing admission; a lagging removal follower; the handshake limit answering `503`; the 64 MiB frame cap; large binary frames arriving byte-identical and still binary; close codes and close handshake in both directions; revoking a pane and removing a host closing live streams; connection caps; handshake timeout; a peer that never reads (write and close deadlines under a paused clock); close codes for each ending. The pinned TLS client is tested against a matching certificate, an impostor, a replayed old certificate and a TLS 1.2 server.

## 6. Residual risk and decisions for the owner

1. **Loopback is not authenticated beyond the token.** Any local process holding a token can use it. Accepted: the same process could use the pane's own transport. Reduce by short TTLs: the default of 12 hours can drop to the pane's session if panes re-request a token on connect (cheap, recommended).
2. **No TLS on loopback.** Accepted for the reason above; a local sniffer is already inside the trust boundary.
3. **Token in a subprotocol needs a small `WebSocket` wrapper in the shell's init script**, because pane code builds URLs as `new URL(path, base)` and the browser API cannot send headers. The wrapper adds the two subprotocols to dials aimed at the relay and nothing else. A bug there fails closed (no token, refused).
4. **Daemon-issued tokens would be stronger.** If the daemon issued per-pane tokens on an authenticated loopback listener, the shell would no longer hold a second credential store and the daemon could enforce scope on its own. `StreamAccess` is the seam: `ShellTransport` and every pane call `issue` and `revoke_pane`, so a daemon-backed implementation replaces the relay without touching callers. Recommended before extensions ship third-party code.
5. **Origin list correctness** is the shell's job: `ExactOrigins` is only as tight as the origins the shell passes. Webviews that load remote content must never share an origin with a core pane.
6. **Descriptor and memory exhaustion** are bounded by the connection caps and the per-message cap, but a hostile local process can still occupy all 256 slots only if it holds valid tokens. Handshake slots are separate and time-limited. A hostile local process can still fill the 64 handshake slots for 3 s at a time and delay a legitimate dial; this is accepted, since the dial retries.
7. **Route prefix.** The extension prefix `/v0/ext/<id>/` follows the protocol draft; change `access_scope::check` if the final routes differ.
8. **Whole-body HTTP answers.** `DaemonResponse` carries a whole body; there is no streaming over `DaemonTransport`. Large downloads must use the WebSocket relay or a future streaming path (`docs/plans/2026-11-14-daemon-response-streaming.md`). Local bodies are capped at 1 GiB, remote at the remote limit.
9. **Tokens on loopback in a subprotocol are visible to a same-user sniffer**, as in item 2.

## 7. Differences from the Tauri originals

- Host id alphabet is lowercase letters, digits and `-` only, as the original; `_` was briefly allowed and is removed. `local` and `localhost` cannot be registered, since they name the local daemon.
- The browser-tab deny rule moved from the proxy into the shell's origin table (see section 4); the transport has no webview labels.
- The first address of a host is no longer tried to exhaustion before the next (staggered dialling, 250 ms), restoring the original's happy-eyeballs behaviour.
- Pairing orders its writes so a failed write never leaves a valid token behind; the original stored the keychain entry before the host list.
- The relay refuses over-limit clients before the upgrade (`503`) instead of upgrading and closing with `Again`.
- The original daemon-respawn handling stays in the daemon supervisor crate, not here.
