# Remote Instances: Tauri Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the desktop app's Rust shell the ability to proxy `crowbar://` HTTP and WebSocket traffic to a REMOTE, TLS-pinned, bearer-authenticated daemon — not just the local unix socket it exclusively talks to today.

**Architecture:** Today `api_proxy.rs`'s `crowbar://` handler and `ws_bridge.rs`'s `ws_open` command each dial exactly one hardcoded target: the local daemon's unix socket. This plan introduces a `Target` the local path already implicitly was (a `Local(PathBuf)` variant) alongside a new `Remote { addr, fingerprint }` variant, dialed over TCP+TLS with a custom certificate verifier that pins to a fingerprint instead of validating a CA chain, with the bearer token attached from the OS keychain. Routing is by the request's authority: `crowbar://localhost/...` (unchanged) stays local; `crowbar://<hostId>/...` is remote. The generalization is a small, additive diff to two existing files — the local path's own tests must keep passing unchanged.

**Tech Stack:** Rust, Tauri 2, `tokio`, `hyper` 1, `tokio-rustls`/`rustls`, `tokio-tungstenite`, `keyring`.

**Spec:** `docs/superpowers/specs/2026-09-27-remote-instances-design.md` (§3 Auth & transport). This plan implements the CLIENT half of §3 — the daemon half is `docs/superpowers/plans/2026-09-27-remote-instances-daemon.md`, already built and independently testable; this plan can be developed against a stub/fixture daemon and integration-verified against the real one once both exist.

## Global Constraints

- `cd desktop/src-tauri && cargo build` must stay clean throughout — do not leave a task in a non-building state.
- Tests: `#[cfg(test)] mod tests { ... }` at the bottom of the file under test, `#[test]` for sync, `#[tokio::test]` for async — mirror `api_proxy.rs`'s and `ws_bridge.rs`'s existing test modules exactly (read them in full before adding to them). Run via `cd desktop/src-tauri && cargo test`.
- `cargo clippy --all-targets` must stay clean (the project runs clippy in CI — check `desktop/Makefile`'s `lint` target for the exact invocation and flags before treating a warning as acceptable).
- New Tauri commands are added to the `tauri::generate_handler![...]` list in `src/lib.rs` — there is no capability/ACL manifest in this app (confirmed: no `permissions/` directory), so a command is reachable the moment it's registered.
- **The existing local (unix-socket) proxy path must not regress.** Every task that touches `api_proxy.rs` or `ws_bridge.rs` ends with running that file's FULL existing test module, not just the new tests.
- No new dependency without adding it to `Cargo.toml` in the task that first needs it and running `cargo build` immediately to confirm it resolves — do not batch dependency additions.

## Review Focus

- A cert presented by a MITM'd or re-installed remote daemon (wrong fingerprint) must hard-fail the TLS handshake — never fall back to accepting it, and never silently retry over plaintext.
- Local (`crowbar://localhost/...`) traffic must be byte-for-byte unaffected by this change — same code path, same tests, same behavior, since almost every request the app makes today is local.
- A remote host with no stored token (never paired, or token was forgotten/revoked) must fail the proxied request cleanly (no panic, no hang) rather than dialing with an empty/missing `Authorization` header and letting the daemon's own 401 be the only signal.
- Removing a host (`unregister_remote_host`) must make its target immediately un-proxyable — no stale cached connection or in-flight retry loop keeps talking to it.
- The WebSocket path (`ws_bridge.rs`) must apply the exact same fingerprint-pinning and bearer-attachment as the HTTP path — a remote host that's rejected over HTTP but silently accepted over WS would be a real, exploitable inconsistency.

---

### Task 1: TLS/keychain dependencies + fingerprint formatting

**Files:**
- Modify: `desktop/src-tauri/Cargo.toml`
- Create: `desktop/src-tauri/src/tls_pin.rs`
- Modify: `desktop/src-tauri/src/lib.rs` (add `mod tls_pin;`)

**Interfaces:**
- Produces: `tls_pin::fingerprint_of(der: &[u8]) -> String` — MUST produce byte-identical output to the Go daemon's `tlsid.FingerprintOf` (`api/internal/core/tlsid/tlsid.go`, Task 1 of the daemon plan) for the same certificate, since a human visually compares the two strings during pairing.

- [ ] **Step 1: Add dependencies**

Add to `desktop/src-tauri/Cargo.toml`'s `[dependencies]` (alongside the existing `tokio-tungstenite = "0.30"`):

```toml
rustls = "0.23"
tokio-rustls = "0.26"
sha2 = "0.10"
keyring = "3"
```

Run: `cd desktop/src-tauri && cargo build 2>&1 | tail -60`
Expected: dependency resolution succeeds (network access required); if any version pin conflicts with an existing transitive dependency, `cargo build`'s error names the conflict — adjust the pinned version to whatever `cargo` reports as compatible and re-run before continuing.

- [ ] **Step 2: Write the failing test**

```rust
// at the bottom of src/tls_pin.rs, once the file has the function below
#[cfg(test)]
mod tests {
    use super::fingerprint_of;

    #[test]
    fn fingerprint_of_is_deterministic() {
        let der = b"some fake certificate bytes for a unit test";
        assert_eq!(fingerprint_of(der), fingerprint_of(der));
    }

    #[test]
    fn fingerprint_of_has_the_go_side_format() {
        // Format contract with api/internal/core/tlsid/tlsid.go's FingerprintOf:
        // "SHA256:" + six groups of 4 lowercase hex chars, space-separated —
        // the first 24 hex chars (12 bytes) of the SHA-256 digest, truncated
        // the same way on both sides so a human can visually compare them.
        let fp = fingerprint_of(b"anything");
        assert!(fp.starts_with("SHA256:"));
        let groups: Vec<&str> = fp["SHA256:".len()..].trim().split(' ').collect();
        assert_eq!(groups.len(), 6, "expected 6 groups, got {:?}", groups);
        for g in groups {
            assert_eq!(g.len(), 4, "expected each group to be 4 hex chars, got {:?}", g);
            assert!(g.chars().all(|c| c.is_ascii_hexdigit()));
        }
    }

    #[test]
    fn fingerprint_of_distinguishes_different_input() {
        assert_ne!(fingerprint_of(b"a"), fingerprint_of(b"b"));
    }
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd desktop/src-tauri && cargo test tls_pin -- --nocapture 2>&1 | tail -40`
Expected: FAIL — `fingerprint_of` undefined / module doesn't compile.

- [ ] **Step 4: Implement**

```rust
//! Fingerprint pinning for remote daemon connections: instead of validating a
//! certificate chain against a CA (there is none — every daemon's identity is
//! self-signed, `api/internal/core/tlsid`), a remote host's certificate is
//! trusted exactly when its fingerprint matches the one the user confirmed
//! during pairing.

use sha2::{Digest, Sha256};

/// Renders a certificate's SHA-256 as grouped lowercase hex — byte-identical
/// in format to the Go daemon's `tlsid.FingerprintOf`, so the string a human
/// reads from `crowbar pair` on the daemon's own host and the string shown
/// here during pairing are directly comparable.
pub fn fingerprint_of(der: &[u8]) -> String {
    let hash = Sha256::digest(der);
    let full: String = hash.iter().map(|b| format!("{b:02x}")).collect();
    let groups: Vec<&str> = (0..6).map(|i| &full[i * 4..(i + 1) * 4]).collect();
    format!("SHA256:{}", groups.join(" "))
}

/// Whether `der`'s fingerprint matches `expected` (as produced by
/// `fingerprint_of` or by the Go daemon's `tlsid.FingerprintOf`).
pub fn fingerprint_matches(der: &[u8], expected: &str) -> bool {
    fingerprint_of(der) == expected
}
```

Add `mod tls_pin;` near the other `mod` declarations in `src/lib.rs` (read the file's existing module list first and match its style/ordering).

- [ ] **Step 5: Run to verify the tests pass**

Run: `cd desktop/src-tauri && cargo test tls_pin -- --nocapture 2>&1 | tail -40`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock desktop/src-tauri/src/tls_pin.rs desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): TLS/keychain deps and a fingerprint formatter matching the daemon's"
```

---

### Task 2: pinned `rustls::ClientConfig` (the actual certificate verifier)

**Files:**
- Modify: `desktop/src-tauri/src/tls_pin.rs`

**Interfaces:**
- Consumes: `fingerprint_matches` (Task 1).
- Produces: `tls_pin::pinned_client_config(fingerprint: String) -> Arc<rustls::ClientConfig>`.

- [ ] **Step 1: Implement the verifier**

This is the one place this plan cannot pre-verify an exact trait signature against a specific `rustls` patch version — `rustls`'s `danger::ServerCertVerifier` trait has had minor signature changes across 0.23.x releases. Write this, then let `cargo build`'s error (if any) name the exact expected signature and adjust — this is a normal, expected part of the step, not a sign the plan is wrong.

```rust
use std::sync::Arc;

use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{DigitallySignedStruct, Error as TlsError, SignatureScheme};

/// Trusts exactly one certificate: the one whose fingerprint was confirmed
/// during pairing. No CA chain, no hostname check — trust-on-first-connect,
/// pinned thereafter.
#[derive(Debug)]
struct PinnedVerifier {
    fingerprint: String,
}

impl ServerCertVerifier for PinnedVerifier {
    fn verify_server_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        _intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, TlsError> {
        if fingerprint_matches(end_entity.as_ref(), &self.fingerprint) {
            Ok(ServerCertVerified::assertion())
        } else {
            Err(TlsError::General(format!(
                "remote host's certificate fingerprint does not match the pinned one \
                 (expected {}) — this host's identity changed",
                self.fingerprint
            )))
        }
    }

    fn verify_tls12_signature(
        &self,
        _message: &[u8],
        _cert: &CertificateDer<'_>,
        _dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        // No chain to validate a signature against — the certificate itself
        // is the trust anchor once its fingerprint has matched above.
        Ok(HandshakeSignatureValid::assertion())
    }

    fn verify_tls13_signature(
        &self,
        _message: &[u8],
        _cert: &CertificateDer<'_>,
        _dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        Ok(HandshakeSignatureValid::assertion())
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        // The daemon's identity is ECDSA P-256 (api/internal/core/tlsid.go
        // generates elliptic.P256 keys) — TLS 1.3 signs with ECDSA-SHA256.
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    }
}

/// Builds a rustls client config that trusts only the certificate matching
/// `fingerprint`.
pub fn pinned_client_config(fingerprint: String) -> Arc<rustls::ClientConfig> {
    let verifier = Arc::new(PinnedVerifier { fingerprint });
    Arc::new(
        rustls::ClientConfig::builder()
            .dangerous()
            .with_custom_certificate_verifier(verifier)
            .with_no_client_auth(),
    )
}
```

- [ ] **Step 2: Build and fix any trait-signature drift**

Run: `cd desktop/src-tauri && cargo build 2>&1 | tail -80`

If the compiler reports a mismatched trait method signature (added/removed parameter, renamed type), open `cargo doc --open -p rustls` or the version's docs.rs page for `rustls::client::danger::ServerCertVerifier`, match the real signature exactly, and rebuild. Do not add `#[allow(...)]` to silence a real mismatch.

- [ ] **Step 3: Write a test proving the verifier actually rejects a mismatch**

This needs a real (if trivial) TLS handshake, so it belongs in this plan's later end-to-end task (Task 5) rather than here as an isolated unit test — a `ServerCertVerifier` cannot be meaningfully unit-tested without a real certificate and a real handshake driving `verify_server_cert`. Confirm this file still builds cleanly and move on; Task 5 is where "wrong fingerprint fails the handshake" gets its test (this is also this plan's #1 Review Focus item — do not skip Task 5's test for it).

- [ ] **Step 4: Commit**

```bash
git add desktop/src-tauri/src/tls_pin.rs
git commit -m "feat(desktop): pinned rustls ClientConfig — trust-on-first-connect, no CA"
```

---

### Task 3: token storage (OS keychain, behind a testable trait)

**Files:**
- Create: `desktop/src-tauri/src/host_tokens.rs`
- Modify: `desktop/src-tauri/src/lib.rs` (add `mod host_tokens;`, register the new commands)

**Interfaces:**
- Produces: `trait TokenStore { fn get(&self, host_id: &str) -> Result<Option<String>, String>; fn set(&self, host_id: &str, token: &str) -> Result<(), String>; fn delete(&self, host_id: &str) -> Result<(), String>; }`, `KeyringTokenStore` (production impl, service name `"crowbar-desktop"`), Tauri commands `store_host_token(host_id: String, token: String) -> Result<(), String>` and `forget_host_token(host_id: String) -> Result<(), String>`.

- [ ] **Step 1: Write the failing tests against a fake store**

```rust
use std::collections::HashMap;
use std::sync::Mutex;

/// TokenStore is the seam host_tokens' Tauri commands and api_proxy's proxy
/// path both go through — production talks to the OS keychain; tests use an
/// in-memory fake, since exercising the real keychain in CI is unreliable
/// (prompts, platform differences) and unnecessary to prove this module's
/// own logic.
pub trait TokenStore: Send + Sync {
    fn get(&self, host_id: &str) -> Result<Option<String>, String>;
    fn set(&self, host_id: &str, token: &str) -> Result<(), String>;
    fn delete(&self, host_id: &str) -> Result<(), String>;
}

#[cfg(test)]
mod tests {
    use super::*;

    struct FakeStore(Mutex<HashMap<String, String>>);
    impl FakeStore {
        fn new() -> Self {
            Self(Mutex::new(HashMap::new()))
        }
    }
    impl TokenStore for FakeStore {
        fn get(&self, host_id: &str) -> Result<Option<String>, String> {
            Ok(self.0.lock().unwrap().get(host_id).cloned())
        }
        fn set(&self, host_id: &str, token: &str) -> Result<(), String> {
            self.0.lock().unwrap().insert(host_id.to_string(), token.to_string());
            Ok(())
        }
        fn delete(&self, host_id: &str) -> Result<(), String> {
            self.0.lock().unwrap().remove(host_id);
            Ok(())
        }
    }

    #[test]
    fn set_then_get_roundtrips() {
        let store = FakeStore::new();
        store.set("host-1", "sometoken").unwrap();
        assert_eq!(store.get("host-1").unwrap(), Some("sometoken".to_string()));
    }

    #[test]
    fn get_missing_host_returns_none() {
        let store = FakeStore::new();
        assert_eq!(store.get("no-such-host").unwrap(), None);
    }

    #[test]
    fn delete_removes_it() {
        let store = FakeStore::new();
        store.set("host-1", "tok").unwrap();
        store.delete("host-1").unwrap();
        assert_eq!(store.get("host-1").unwrap(), None);
    }
}
```

- [ ] **Step 2: Run to verify failure, then implement the trait + keyring-backed impl**

```rust
//! Bearer tokens for remote hosts, in the OS keychain — never in the
//! tauri-plugin-store config JSON alongside a host's name/endpoint/
//! fingerprint (that store is client-visible app config; a bearer token is a
//! secret).

const SERVICE: &str = "crowbar-desktop";

pub struct KeyringTokenStore;

impl TokenStore for KeyringTokenStore {
    fn get(&self, host_id: &str) -> Result<Option<String>, String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        match entry.get_password() {
            Ok(token) => Ok(Some(token)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(e.to_string()),
        }
    }

    fn set(&self, host_id: &str, token: &str) -> Result<(), String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        entry.set_password(token).map_err(|e| e.to_string())
    }

    fn delete(&self, host_id: &str) -> Result<(), String> {
        let entry = keyring::Entry::new(SERVICE, host_id).map_err(|e| e.to_string())?;
        match entry.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(e) => Err(e.to_string()),
        }
    }
}
```

Run: `cd desktop/src-tauri && cargo test host_tokens -- --nocapture 2>&1 | tail -40`
Expected: PASS. If `keyring::Entry::new`/`delete_credential`'s exact signature differs from this plan's assumption for the pinned `keyring = "3"` version, fix from the compiler error the same way Task 2 did.

- [ ] **Step 3: Write the Tauri commands**

```rust
#[tauri::command]
pub fn store_host_token(
    host_id: String,
    token: String,
    store: tauri::State<'_, Box<dyn TokenStore>>,
) -> Result<(), String> {
    store.set(&host_id, &token)
}

#[tauri::command]
pub fn forget_host_token(
    host_id: String,
    store: tauri::State<'_, Box<dyn TokenStore>>,
) -> Result<(), String> {
    store.delete(&host_id)
}
```

Register `Box::new(KeyringTokenStore) as Box<dyn TokenStore>` via `.manage(...)` in `src/lib.rs`'s `run()`, alongside wherever `SidecarHandle`/`WsBridgeManager` are already managed (read that section of `lib.rs` first and mirror its exact pattern). Add `store_host_token` and `forget_host_token` to the `tauri::generate_handler![...]` list.

- [ ] **Step 4: Build and test**

Run: `cd desktop/src-tauri && cargo build && cargo test host_tokens -- --nocapture 2>&1 | tail -60`
Expected: clean build, tests pass.

- [ ] **Step 5: Commit**

```bash
git add desktop/src-tauri/src/host_tokens.rs desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): OS-keychain-backed bearer token storage per host"
```

---

### Task 4: the host registry (endpoint + pinned fingerprint, in memory)

**Files:**
- Create: `desktop/src-tauri/src/hosts.rs`
- Modify: `desktop/src-tauri/src/lib.rs` (add `mod hosts;`, register commands + managed state)

**Interfaces:**
- Produces: `hosts::RemoteHost{endpoint: String, fingerprint: String}`, `hosts::HostRegistry` (a `Mutex<HashMap<String, RemoteHost>>` newtype with `get`/`insert`/`remove`), Tauri commands `register_remote_host(host_id, endpoint, fingerprint) -> Result<(), String>`, `unregister_remote_host(host_id) -> Result<(), String>`.

- [ ] **Step 1: Write the failing tests**

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn insert_then_get_roundtrips() {
        let reg = HostRegistry::default();
        reg.insert("host-1".into(), RemoteHost { endpoint: "1.2.3.4:3737".into(), fingerprint: "SHA256:aaaa bbbb cccc dddd eeee ffff".into() });
        let got = reg.get("host-1").expect("expected host-1 to be registered");
        assert_eq!(got.endpoint, "1.2.3.4:3737");
    }

    #[test]
    fn get_unknown_host_returns_none() {
        let reg = HostRegistry::default();
        assert!(reg.get("no-such-host").is_none());
    }

    #[test]
    fn remove_makes_it_unreachable() {
        // Review Focus: removing a host must make it immediately un-proxyable.
        let reg = HostRegistry::default();
        reg.insert("host-1".into(), RemoteHost { endpoint: "1.2.3.4:3737".into(), fingerprint: "fp".into() });
        reg.remove("host-1");
        assert!(reg.get("host-1").is_none());
    }
}
```

- [ ] **Step 2: Run to verify failure, then implement**

```rust
//! In-memory registry of remote hosts this session knows how to dial:
//! endpoint + pinned fingerprint. This is a RUNTIME cache the web layer
//! populates at startup and on every host add/remove/edit (its own
//! persistence — likely tauri-plugin-store — owns the durable copy; this
//! registry only ever answers "where and how do I dial host X right now").
//! Bearer tokens are NOT here — see host_tokens.rs.

use std::collections::HashMap;
use std::sync::Mutex;

#[derive(Clone, Debug)]
pub struct RemoteHost {
    pub endpoint: String,
    pub fingerprint: String,
}

#[derive(Default)]
pub struct HostRegistry(Mutex<HashMap<String, RemoteHost>>);

impl HostRegistry {
    pub fn get(&self, host_id: &str) -> Option<RemoteHost> {
        self.0.lock().unwrap().get(host_id).cloned()
    }

    pub fn insert(&self, host_id: String, host: RemoteHost) {
        self.0.lock().unwrap().insert(host_id, host);
    }

    pub fn remove(&self, host_id: &str) {
        self.0.lock().unwrap().remove(host_id);
    }
}

#[tauri::command]
pub fn register_remote_host(
    host_id: String,
    endpoint: String,
    fingerprint: String,
    registry: tauri::State<'_, HostRegistry>,
) -> Result<(), String> {
    registry.insert(host_id, RemoteHost { endpoint, fingerprint });
    Ok(())
}

#[tauri::command]
pub fn unregister_remote_host(
    host_id: String,
    registry: tauri::State<'_, HostRegistry>,
) -> Result<(), String> {
    registry.remove(&host_id);
    Ok(())
}
```

Register `HostRegistry::default()` via `.manage(...)` in `lib.rs`, and add both commands to `generate_handler![...]`.

- [ ] **Step 3: Build and test**

Run: `cd desktop/src-tauri && cargo build && cargo test hosts:: -- --nocapture 2>&1 | tail -60`
Expected: clean build, tests pass.

- [ ] **Step 4: Commit**

```bash
git add desktop/src-tauri/src/hosts.rs desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): in-memory remote-host registry (endpoint + pinned fingerprint)"
```

---

### Task 5: `api_proxy.rs` — route by authority, dial remote over pinned TLS

**Files:**
- Modify: `desktop/src-tauri/src/api_proxy.rs` (read the ENTIRE current file first — this plan's earlier research quotes most of it, but re-read before editing; it is the most delicate file in this plan)

**Interfaces:**
- Consumes: `tls_pin::pinned_client_config` (Task 2), `HostRegistry` (Task 4), `TokenStore` (Task 3).
- Produces: `handle_request` unchanged in signature; `forward` generalized to a `Target` enum instead of `Fn() -> Option<PathBuf>`.

- [ ] **Step 1: Write the failing test for remote routing + fingerprint rejection**

Add to `api_proxy.rs`'s existing `#[cfg(test)] mod tests` block (do not replace it — extend it; every existing test in that block must still pass unmodified after this task):

```rust
    // New imports for this task's tests, added to the existing use block above:
    // tokio_rustls::TlsAcceptor, rustls::pki_types::{CertificateDer, PrivateKeyDer}
    // and this crate's tls_pin/hosts/host_tokens modules.

    /// Builds a minimal self-signed TLS identity for a test TCP server, using
    /// the same generation approach as api/internal/core/tlsid (self-signed,
    /// ECDSA P-256) — via the `rcgen` dev-dependency (add
    /// `rcgen = "0.13"` under `[dev-dependencies]` in Cargo.toml for this
    /// task if it is not already present).
    fn test_tls_identity() -> (rustls::pki_types::CertificateDer<'static>, rustls::pki_types::PrivateKeyDer<'static>) {
        let cert = rcgen::generate_simple_self_signed(vec!["localhost".into()]).unwrap();
        let der = cert.cert.der().clone();
        let key = rustls::pki_types::PrivateKeyDer::Pkcs8(cert.key_pair.serialize_der().into());
        (der, key)
    }

    struct FakeTokenStore(String);
    impl crate::host_tokens::TokenStore for FakeTokenStore {
        fn get(&self, _host_id: &str) -> Result<Option<String>, String> { Ok(Some(self.0.clone())) }
        fn set(&self, _: &str, _: &str) -> Result<(), String> { Ok(()) }
        fn delete(&self, _: &str) -> Result<(), String> { Ok(()) }
    }

    /// Binds a TLS-terminated TCP listener and answers exactly one HTTP/1.1
    /// request with 200, asserting the Authorization header it received.
    async fn serve_once_tls(cert: rustls::pki_types::CertificateDer<'static>, key: rustls::pki_types::PrivateKeyDer<'static>, expected_auth: &'static str) -> std::net::SocketAddr {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let server_config = rustls::ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(vec![cert], key)
            .unwrap();
        let acceptor = tokio_rustls::TlsAcceptor::from(std::sync::Arc::new(server_config));
        tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let mut tls = acceptor.accept(stream).await.unwrap();
            let mut buf = vec![0u8; 4096];
            let mut read = 0;
            loop {
                let n = tokio::io::AsyncReadExt::read(&mut tls, &mut buf[read..]).await.unwrap();
                read += n;
                if n == 0 || buf[..read].windows(4).any(|w| w == b"\r\n\r\n") { break; }
            }
            let req = String::from_utf8_lossy(&buf[..read]);
            assert!(req.contains(expected_auth), "expected request to carry {expected_auth}, got: {req}");
            tokio::io::AsyncWriteExt::write_all(&mut tls, b"HTTP/1.1 200 OK\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok").await.unwrap();
        });
        addr
    }

    #[tokio::test]
    async fn remote_request_carries_the_bearer_token_and_succeeds() {
        let (cert, key) = test_tls_identity();
        let fingerprint = crate::tls_pin::fingerprint_of(cert.as_ref());
        let addr = serve_once_tls(cert, key, "Authorization: Bearer sometoken").await;

        let registry = HostRegistry::default();
        registry.insert("host-1".into(), crate::hosts::RemoteHost { endpoint: addr.to_string(), fingerprint });
        let tokens: std::sync::Arc<dyn crate::host_tokens::TokenStore> = std::sync::Arc::new(FakeTokenStore("sometoken".into()));

        let req = http::Request::builder()
            .method(http::Method::GET)
            .uri("crowbar://host-1/v0/projects")
            .body(Vec::new())
            .unwrap();

        let resp = forward_remote(&registry, &tokens, "host-1", req).await;
        assert_eq!(resp.status(), 200);
    }

    #[tokio::test]
    async fn remote_request_wrong_fingerprint_fails_the_handshake() {
        // Review Focus: a fingerprint mismatch must hard-fail, never fall
        // back to accepting the connection.
        let (cert, key) = test_tls_identity();
        let addr = serve_once_tls(cert, key, "Authorization: Bearer sometoken").await;

        let registry = HostRegistry::default();
        registry.insert("host-1".into(), crate::hosts::RemoteHost {
            endpoint: addr.to_string(),
            fingerprint: "SHA256:0000 0000 0000 0000 0000 0000".into(), // deliberately wrong
        });
        let tokens: std::sync::Arc<dyn crate::host_tokens::TokenStore> = std::sync::Arc::new(FakeTokenStore("sometoken".into()));

        let req = http::Request::builder()
            .method(http::Method::GET)
            .uri("crowbar://host-1/v0/projects")
            .body(Vec::new())
            .unwrap();

        let resp = forward_remote(&registry, &tokens, "host-1", req).await;
        assert_ne!(resp.status(), 200, "expected the wrong-fingerprint handshake to fail, not return 200");
    }

    #[tokio::test]
    async fn remote_request_no_stored_token_fails_cleanly() {
        // Review Focus: a host with no token must not hang or panic.
        let registry = HostRegistry::default();
        registry.insert("host-1".into(), crate::hosts::RemoteHost { endpoint: "127.0.0.1:1".into(), fingerprint: "SHA256:x".into() });
        struct NoTokenStore;
        impl crate::host_tokens::TokenStore for NoTokenStore {
            fn get(&self, _: &str) -> Result<Option<String>, String> { Ok(None) }
            fn set(&self, _: &str, _: &str) -> Result<(), String> { Ok(()) }
            fn delete(&self, _: &str) -> Result<(), String> { Ok(()) }
        }
        let tokens: std::sync::Arc<dyn crate::host_tokens::TokenStore> = std::sync::Arc::new(NoTokenStore);

        let req = http::Request::builder().method(http::Method::GET).uri("crowbar://host-1/v0/x").body(Vec::new()).unwrap();
        let resp = tokio::time::timeout(Duration::from_secs(2), forward_remote(&registry, &tokens, "host-1", req)).await
            .expect("forward_remote must not hang when no token is stored");
        assert_ne!(resp.status(), 200);
    }
```

- [ ] **Step 2: Run to verify failure**

Run: `cd desktop/src-tauri && cargo test api_proxy -- --nocapture 2>&1 | tail -100`
Expected: FAIL — `forward_remote`, `HostRegistry` (as used here without the `hosts::` prefix — add the import), etc. undefined or not yet wired.

- [ ] **Step 3: Implement — generalize `forward`/`proxy`, add `forward_remote`, update `handle_request`**

Read the CURRENT `forward` and `proxy` functions in full before editing (this plan's earlier research quotes them; re-confirm against the live file, since Task 1–4 of this plan did not touch this file and it should be unchanged from that quote). Make these changes:

1. Generalize `proxy` to be generic over the stream type instead of hardcoded to `UnixStream`:

```rust
async fn proxy<S>(stream: S, request: http::Request<Vec<u8>>) -> Result<http::Response<Vec<u8>>, Box<dyn std::error::Error + Send + Sync>>
where
    S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send + 'static,
{
    // body unchanged from today — only the `stream: UnixStream` parameter
    // type becomes generic; TokioIo::new(stream) already accepts any
    // AsyncRead+AsyncWrite, so nothing inside this function's body changes.
    ...
}
```

2. Add `forward_remote`, dialing via `tokio::net::TcpStream` + `tokio_rustls::TlsConnector` built from `tls_pin::pinned_client_config`, attaching the bearer token as a request header before calling the now-generic `proxy`:

```rust
async fn forward_remote(
    registry: &HostRegistry,
    tokens: &std::sync::Arc<dyn crate::host_tokens::TokenStore>,
    host_id: &str,
    mut request: http::Request<Vec<u8>>,
) -> http::Response<Vec<u8>> {
    let host = match registry.get(host_id) {
        Some(h) => h,
        None => return error_response(502, "unknown remote host"),
    };
    let token = match tokens.get(host_id) {
        Ok(Some(t)) => t,
        Ok(None) => return error_response(401, "no stored credential for this host — pair it again"),
        Err(e) => return error_response(500, &format!("token lookup failed: {e}")),
    };
    request.headers_mut().insert(
        http::header::AUTHORIZATION,
        match http::HeaderValue::from_str(&format!("Bearer {token}")) {
            Ok(v) => v,
            Err(e) => return error_response(500, &format!("invalid token: {e}")),
        },
    );

    let tcp = match tokio::net::TcpStream::connect(&host.endpoint).await {
        Ok(s) => s,
        Err(e) => return error_response(502, &format!("cannot reach {}: {e}", host.endpoint)),
    };
    let connector = tokio_rustls::TlsConnector::from(crate::tls_pin::pinned_client_config(host.fingerprint));
    let domain = match rustls::pki_types::ServerName::try_from("crowbar-remote".to_string()) {
        // The server name is meaningless here (verification is fingerprint-
        // based, not hostname-based — PinnedVerifier ignores it entirely) but
        // rustls' API still requires a syntactically valid one.
        Ok(n) => n,
        Err(e) => return error_response(500, &format!("internal: {e}")),
    };
    let tls_stream = match connector.connect(domain, tcp).await {
        Ok(s) => s,
        Err(e) => return error_response(502, &format!("TLS handshake with {} failed: {e}", host.endpoint)),
    };

    match proxy(tls_stream, request).await {
        Ok(resp) => resp,
        Err(e) => error_response(502, &format!("crowbar proxy error: {e}")),
    }
}
```

3. Update `handle_request` to branch on the request URI's authority:

```rust
pub fn handle_request<R: Runtime>(
    ctx: UriSchemeContext<'_, R>,
    request: http::Request<Vec<u8>>,
    responder: UriSchemeResponder,
) {
    let app = ctx.app_handle().clone();
    let host_authority = request.uri().host().unwrap_or("").to_string();

    tauri::async_runtime::spawn(async move {
        let resp = if host_authority.is_empty() || host_authority == "localhost" {
            // Unchanged local path.
            let state_app = app.clone();
            let forwarded = forward(
                || state_app.state::<SidecarHandle>().socket_path(),
                request,
                |attempt| tokio::time::sleep(connect_retry_delay(attempt)),
            );
            match tokio::time::timeout(PROXY_TIMEOUT, forwarded).await {
                Ok(resp) => resp,
                Err(_) => error_response(504, "crowbar daemon did not answer in time"),
            }
        } else {
            let registry = app.state::<HostRegistry>();
            let tokens = app.state::<std::sync::Arc<dyn crate::host_tokens::TokenStore>>();
            match tokio::time::timeout(PROXY_TIMEOUT, forward_remote(&registry, &tokens, &host_authority, request)).await {
                Ok(resp) => resp,
                Err(_) => error_response(504, "remote crowbar daemon did not answer in time"),
            }
        };
        let _ = app.run_on_main_thread(move || responder.respond(resp));
    });
}
```

Check whether `HostRegistry`/the token store are already managed as `Box<dyn TokenStore>` (Task 3) vs `Arc<dyn TokenStore>` (used here) — pick ONE wrapper type and use it consistently across Tasks 3 and 5; if Task 3 used `Box`, change either that task's `.manage(...)` call or this one so they match (an `Arc` is the better choice here since `forward_remote` needs to hold a cloned reference across an `await` inside a spawned task — prefer changing Task 3 to `Arc` if there's a mismatch).

Add `use crate::hosts::HostRegistry;` and any other needed imports at the top of `api_proxy.rs`.

- [ ] **Step 4: Run the FULL existing test module plus the new tests**

Run: `cd desktop/src-tauri && cargo test api_proxy -- --nocapture 2>&1 | tail -150`
Expected: every pre-existing test in this file's `mod tests` (Task 5's own Review Focus item: local path unaffected) plus all three new tests PASS.

- [ ] **Step 5: `cargo clippy` and full build**

Run: `cd desktop/src-tauri && cargo clippy --all-targets 2>&1 | tail -80 && cargo build`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add desktop/src-tauri/src/api_proxy.rs desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock
git commit -m "feat(desktop): proxy HTTP to a remote, TLS-pinned, bearer-authed daemon"
```

---

**Addendum to Task 5 (added post-implementation, during review):** Task 5's own review found that the original plan had no way for a client to redeem a pairing code at all — `forward_remote` correctly refuses to dial any host with no stored token, but pairing IS how a token gets created, so every pairing attempt would 401 before ever reaching the daemon. The fix, now shipped as part of Task 5: a dedicated `pair_remote_host(host_id, endpoint, fingerprint, code, name) -> Result<(), String>` command (`desktop/src-tauri/src/api_proxy/pairing.rs`) that dials the pinned connection directly (bypassing the token gate, since none exists yet), sends `POST /v0/pair` itself, and writes the resulting token straight to the keychain — the raw token never crosses into JS. Also shipped in the same fix round: `x-crowbar-proxy: host-identity-changed` (502) vs `host-unreachable` (503) to distinguish a pin-mismatch rejection from a plain network outage, and a 5s connect+handshake deadline separate from the existing 300s whole-request `PROXY_TIMEOUT`.

---

### Task 5b: `probe_remote_fingerprint` — discover a host's fingerprint for TOFU confirmation

**Why this task exists:** `pair_remote_host` (Task 5's addendum, above) takes `fingerprint` as an ALREADY-KNOWN input — but nothing lets the user actually discover it in the first place. The design spec's pairing dialog (§5.3 of the design spec, and the approved mockup) shows the user the presented fingerprint so they can visually confirm it matches what `crowbar pair`'s CLI output printed, BEFORE the code is redeemed. Without this command, the web plan's pairing dialog has no way to reach stage 1 (address entered → fingerprint shown for confirmation) at all.

**Files:**
- Create: `desktop/src-tauri/src/api_proxy/probe.rs` (or add to `pairing.rs` if that reads more naturally — implementer's call, given both files will be small)
- Test: same file, `#[cfg(test)] mod tests`

**Interfaces:**
- Produces: `probe_remote_fingerprint(endpoint: String) -> Result<String, String>` — a new Tauri command, registered in `generate_handler!`.
- Consumes: nothing from `HostRegistry`/`TokenStore` — this command deliberately does NOT touch either, since the host isn't registered and there's no token yet.

**This is the one and only place in the whole codebase where an UNPINNED TLS dial is correct.** Every other remote dial (`forward_remote`, `pair_remote_host`) MUST use the pinned verifier from `tls_pin.rs` — this command is the sole, deliberate exception, because discovering the fingerprint for a human to confirm IS the trust-bootstrap step that pinning depends on. Do not let this dial share any code path with the pinned connectors in a way that could accidentally make it easier to introduce an unpinned dial elsewhere.

- [ ] **Step 1: Write the failing tests**

Write a test proving: (a) dialing a real TLS test server (self-signed cert via `rcgen`, matching the style already established in `tls_pin.rs`/`api_proxy/remote.rs`'s tests) returns exactly that certificate's fingerprint, formatted via the existing `tls_pin::fingerprint_of` (byte-identical to the Go daemon's own format — reuse it, don't reimplement); (b) an unreachable endpoint returns a clear `Err`, not a hang (bound this dial with the same short deadline pattern `forward_remote`/`pair_remote_host` already use — a fingerprint probe that hangs indefinitely on a dead host is exactly the kind of thing this plan has already had to fix twice); (c) the command never registers anything in `HostRegistry` and never touches `TokenStore` (a probe must be side-effect-free besides the network dial itself).

- [ ] **Step 2: Implement**

Build a `rustls::ClientConfig` using rustls's own no-op/accept-all `ServerCertVerifier` for this ONE purpose (check `rustls::client::danger::ServerCertVerifier` for whether a bundled "dangerous, accept everything" implementation already exists in this rustls version before writing your own — if you write your own, name it something that makes its danger unmistakable, e.g. `UnpinnedProbeVerifier`, with a doc comment explaining exactly why it's safe HERE and nowhere else). Dial, complete the handshake, extract the peer's presented certificate's DER bytes from the completed connection, and return `tls_pin::fingerprint_of(der)`.

- [ ] **Step 3: Register and verify**

Add to `generate_handler!` in `lib.rs`. Run the full test suite, `cargo clippy -- -D warnings`, `cargo fmt --check`. Confirm nothing in `remote.rs`/`pairing.rs`'s pinned dial paths was touched.

- [ ] **Step 4: Commit**

```bash
git add desktop/src-tauri/src/api_proxy/ desktop/src-tauri/src/lib.rs
git commit -m "feat(desktop): probe a remote host's TLS fingerprint for pairing confirmation"
```

---

### Task 6: `ws_bridge.rs` — same routing for WebSocket

**Files:**
- Modify: `desktop/src-tauri/src/ws_bridge.rs` (read the ENTIRE current file first)

**Interfaces:**
- Consumes: `HostRegistry` (Task 4), `TokenStore` (Task 3), `tls_pin::pinned_client_config` (Task 2).
- Produces: `ws_open`'s signature gains `host_id: Option<String>`.

- [ ] **Step 1: Write the failing test**

First, read `ws_bridge.rs`'s existing test(s) that assert a received frame end-to-end (the `Channel<InvokeResponseBody>` harness — likely a fake/recording `FrameSink` or a real `Channel` wired to an `mpsc` receiver). The test below is written against this plan's best understanding of that harness from `open_bridge`'s documented shape (reader task forwards each frame to `on_message`); true up the exact harness type/assertion calls against the live file's own existing tests before finalizing — the connect/upgrade/assert-auth-header logic is the new, load-bearing part of this test and should not need to change.

Add to `ws_bridge.rs`'s existing `#[cfg(test)] mod tests` block (extend, do not replace):

```rust
    use tokio_tungstenite::tungstenite::handshake::server::{Request as WsRequest, Response as WsResponse, ErrorResponse};

    fn self_signed_tls_identity() -> (rustls::pki_types::CertificateDer<'static>, rustls::pki_types::PrivateKeyDer<'static>) {
        // Same construction as api_proxy.rs's test_tls_identity — duplicated
        // here deliberately (test-only code, not worth a shared prod module).
        // Requires `rcgen = "0.13"` under [dev-dependencies] (added by Task 5
        // if not already present when this task runs).
        let cert = rcgen::generate_simple_self_signed(vec!["localhost".into()]).unwrap();
        let der = cert.cert.der().clone();
        let key = rustls::pki_types::PrivateKeyDer::Pkcs8(cert.key_pair.serialize_der().into());
        (der, key)
    }

    #[tokio::test]
    async fn ws_open_with_host_id_dials_remote_over_pinned_tls_with_bearer_auth() {
        let (cert, key) = self_signed_tls_identity();
        let fingerprint = crate::tls_pin::fingerprint_of(cert.as_ref());

        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let server_config = rustls::ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(vec![cert], key)
            .unwrap();
        let acceptor = tokio_rustls::TlsAcceptor::from(std::sync::Arc::new(server_config));

        let server = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let tls_stream = acceptor.accept(stream).await.unwrap();
            let mut saw_auth_header = false;
            let callback = |req: &WsRequest, resp: WsResponse| -> Result<WsResponse, ErrorResponse> {
                saw_auth_header = req.headers().get("authorization").map(|v| v.as_bytes()) == Some(b"Bearer sometoken");
                Ok(resp)
            };
            let mut ws = tokio_tungstenite::accept_hdr_async(tls_stream, callback).await.unwrap();
            assert!(saw_auth_header, "expected the WS upgrade request to carry the bearer token");
            futures_util::SinkExt::send(&mut ws, tokio_tungstenite::tungstenite::Message::Text("hello-remote".into())).await.unwrap();
        });

        let registry = HostRegistry::default();
        registry.insert("host-1".into(), crate::hosts::RemoteHost { endpoint: addr.to_string(), fingerprint });
        let tokens: std::sync::Arc<dyn crate::host_tokens::TokenStore> = std::sync::Arc::new(FakeTokenStore("sometoken".into()));
        let manager = WsBridgeManager::default(); // confirm this constructs the way this file's own existing tests already build one

        // on_message / channel plumbing: replace this line with however this
        // file's existing local-path test already captures received frames
        // (a recording channel / mpsc pair) — the assertion below is the
        // part that must hold once wired up correctly.
        let (on_message, mut received) = test_channel(); // defined by / copied from this file's existing test harness

        open_remote_bridge(&registry, &tokens, "host-1", "conn-1".into(), "/v0/ws".into(), on_message, &manager, "test-window".into(), None)
            .await
            .expect("open_remote_bridge should succeed against a valid pinned TLS server");

        let frame = tokio::time::timeout(Duration::from_secs(2), received.recv()).await
            .expect("expected a frame within 2s")
            .expect("channel closed before a frame arrived");
        assert!(format!("{frame:?}").contains("hello-remote"));

        server.await.unwrap();
    }
```

`futures_util` is already a direct dependency (Cargo.toml). Add `rcgen = "0.13"` under `[dev-dependencies]` in this task if Task 5 has not already added it (check first — do not add it twice).

- [ ] **Step 2: Run to verify failure**

Run: `cd desktop/src-tauri && cargo test ws_bridge -- --nocapture 2>&1 | tail -100`
Expected: FAIL.

- [ ] **Step 3: Implement — thread `host_id` through `ws_open`/`open_bridge`**

```rust
#[tauri::command]
pub async fn ws_open(
    conn_id: String,
    path: String,
    host_id: Option<String>,
    on_message: Channel<InvokeResponseBody>,
    idle_timeout_ms: Option<u64>,
    manager: State<'_, WsBridgeManager>,
    registry: State<'_, HostRegistry>,
    tokens: State<'_, std::sync::Arc<dyn crate::host_tokens::TokenStore>>,
    window: tauri::WebviewWindow,
) -> Result<(), String> {
    match host_id.filter(|h| !h.is_empty() && h != "localhost") {
        None => {
            let socket = crate::sidecar::socket_path();
            open_bridge(&socket, conn_id, path, on_message, &manager,
                window.label().to_string(), idle_timeout_ms.map(Duration::from_millis)).await.map(|_reader| ())
        }
        Some(host) => {
            open_remote_bridge(&registry, &tokens, &host, conn_id, path, on_message, &manager,
                window.label().to_string(), idle_timeout_ms.map(Duration::from_millis)).await
        }
    }
}
```

Write `open_remote_bridge` mirroring `open_bridge`'s existing structure (dial → `into_client_request()` → upgrade → split into read/write tasks → register in `WsBridgeManager.connections`), with two differences: the dial is `TcpStream::connect(&host.endpoint)` wrapped in `TlsConnector::connect` (Task 5's `forward_remote` shows the exact connect sequence — reuse the same pattern) instead of `UnixStream::connect`, upgraded via `tokio_tungstenite::client_async_tls` (not the plain `client_async` the local path uses) over that TLS stream; and the upgrade request built via `into_client_request()` gets an `Authorization: Bearer <token>` header inserted before the upgrade, read from `tokens.get(host)` the same way `forward_remote` does (fail cleanly — return `Err(...)`, do not hang — if no token is stored, mirroring Task 5's Review Focus item for this path too).

Read `open_bridge`'s exact current body before writing `open_remote_bridge` — this plan's research summarized it but did not quote it verbatim in full, and the reader/writer task split, cancellation, and `WsBridgeManager` registration logic must match exactly or the two code paths will diverge in ways a future local-only bugfix could silently miss for remote.

- [ ] **Step 4: Run the FULL existing test module plus the new test**

Run: `cd desktop/src-tauri && cargo test ws_bridge -- --nocapture 2>&1 | tail -150`
Expected: every pre-existing test PASSES unmodified, plus the new remote test.

- [ ] **Step 5: `cargo clippy` and full build**

Run: `cd desktop/src-tauri && cargo clippy --all-targets 2>&1 | tail -80 && cargo build`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add desktop/src-tauri/src/ws_bridge.rs
git commit -m "feat(desktop): route WebSocket upgrades to a remote host the same way HTTP is"
```

---

### Task 7: thin JS wrapper (`crowbar-bridge.ts`)

**Files:**
- Modify: `web/src/lib/crowbar-bridge.ts` (read in full first)
- Test: `web/src/__tests__/lib/crowbar-bridge.test.ts` (extend if it exists; check first — the file may not exist yet, in which case create it, mirroring the test style of a sibling `lib/` test file)

**Interfaces:**
- Produces: `registerRemoteHost(hostId: string, endpoint: string, fingerprint: string): Promise<void>`, `unregisterRemoteHost(hostId: string): Promise<void>`, `storeHostToken(hostId: string, token: string): Promise<void>`, `forgetHostToken(hostId: string): Promise<void>` — each a thin `tauriInvoke('<command>', {...})` wrapper, matching this file's existing `tauriInvoke` usage pattern exactly (read an existing wrapped command in this file first, e.g. how `reveal_in_finder` or `popup_native_context_menu` is called, and copy that exact shape).

- [ ] **Step 1: Write the failing tests**

Mirror whatever mocking pattern this repo's existing `crowbar-bridge` or `tauriInvoke`-adjacent tests already use for asserting an invoke call's name and args (search `web/src/__tests__/` for an existing test that mocks `@tauri-apps/api/core`'s `invoke` before writing this — do not invent a new mocking approach if one is already established).

```ts
import { describe, expect, it, vi } from 'vitest'

const invokeMock = vi.fn()
vi.mock('@tauri-apps/api/core', () => ({ invoke: (...args: unknown[]) => invokeMock(...args) }))

import { registerRemoteHost, unregisterRemoteHost, storeHostToken, forgetHostToken } from '@/lib/crowbar-bridge'

describe('remote host bridge commands', () => {
  it('registerRemoteHost invokes register_remote_host with the right args', async () => {
    await registerRemoteHost('host-1', '1.2.3.4:3737', 'SHA256:aaaa')
    expect(invokeMock).toHaveBeenCalledWith('register_remote_host', {
      hostId: 'host-1',
      endpoint: '1.2.3.4:3737',
      fingerprint: 'SHA256:aaaa',
    })
  })

  it('unregisterRemoteHost invokes unregister_remote_host', async () => {
    await unregisterRemoteHost('host-1')
    expect(invokeMock).toHaveBeenCalledWith('unregister_remote_host', { hostId: 'host-1' })
  })

  it('storeHostToken invokes store_host_token', async () => {
    await storeHostToken('host-1', 'sometoken')
    expect(invokeMock).toHaveBeenCalledWith('store_host_token', { hostId: 'host-1', token: 'sometoken' })
  })

  it('forgetHostToken invokes forget_host_token', async () => {
    await forgetHostToken('host-1')
    expect(invokeMock).toHaveBeenCalledWith('forget_host_token', { hostId: 'host-1' })
  })
})
```

Adjust the mock/import shape to match whatever this repo's existing convention actually is once you've read a real example — this is a sketch of the assertions, not a guarantee of the exact mocking mechanics.

- [ ] **Step 2: Run to verify failure**

Run: `cd web && bun vitest run src/__tests__/lib/crowbar-bridge.test.ts 2>&1 | tail -60`
Expected: FAIL — the four functions are undefined.

- [ ] **Step 3: Implement**

```ts
export async function registerRemoteHost(hostId: string, endpoint: string, fingerprint: string): Promise<void> {
  await tauriInvoke('register_remote_host', { hostId, endpoint, fingerprint })
}

export async function unregisterRemoteHost(hostId: string): Promise<void> {
  await tauriInvoke('unregister_remote_host', { hostId })
}

export async function storeHostToken(hostId: string, token: string): Promise<void> {
  await tauriInvoke('store_host_token', { hostId, token })
}

export async function forgetHostToken(hostId: string): Promise<void> {
  await tauriInvoke('forget_host_token', { hostId })
}
```

Place these beside the file's other Tauri-command wrappers, following its existing grouping/section-comment style. Confirm `tauriInvoke`'s exact exported signature by reading its definition in this same file before calling it — this plan's earlier research names it but did not quote its body.

- [ ] **Step 4: Run to verify the tests pass**

Run: `cd web && bun vitest run src/__tests__/lib/crowbar-bridge.test.ts 2>&1 | tail -60`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/crowbar-bridge.ts web/src/__tests__/lib/crowbar-bridge.test.ts
git commit -m "feat(web): thin bridge wrappers for the remote-host Tauri commands"
```

---

### Task 8: full-branch gate

- [ ] **Step 1: Rust gate**

Run: `cd desktop/src-tauri && cargo build && cargo test && cargo clippy --all-targets 2>&1 | tail -150`
Expected: clean build, every test passes (old and new), no clippy warnings on touched code.

- [ ] **Step 2: Web gate (just this task's own file)**

Run: `cd web && bunx tsc --noEmit && bunx eslint src/lib/crowbar-bridge.ts src/__tests__/lib/crowbar-bridge.test.ts --max-warnings 0`
Expected: clean.

- [ ] **Step 3: Fix anything the gates surface, then commit**
