use std::collections::HashMap;

use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use protocol::{HostId, PaneId, RelayScope};
use sha2::{Digest, Sha256};
use tokio::time::{Duration, Instant};

use crate::error::Error;
use crate::fingerprint::DIGEST_BYTES;

const TOKEN_BYTES: usize = 32;

type Digest32 = [u8; DIGEST_BYTES];

pub(crate) struct TokenBook {
    entries: HashMap<Digest32, (RelayScope, Instant)>,
    ttl: Duration,
}

impl TokenBook {
    pub(crate) fn new(ttl: Duration) -> Self {
        Self {
            entries: HashMap::new(),
            ttl,
        }
    }

    pub(crate) fn issue(
        &mut self,
        scope: RelayScope,
        now: Instant,
    ) -> Result<String, Error> {
        let mut raw = [0u8; TOKEN_BYTES];
        getrandom::fill(&mut raw).map_err(|e| Error::Random(e.to_string()))?;
        let token = URL_SAFE_NO_PAD.encode(raw);
        self.entries
            .retain(|_, (held, expires)| *expires > now && !same_stream(held, &scope));
        self.entries.insert(digest(&token), (scope, now + self.ttl));
        Ok(token)
    }

    pub(crate) fn authorize(
        &mut self,
        token: &str,
        now: Instant,
    ) -> Option<RelayScope> {
        let key = digest(token);
        let (scope, expires) = self.entries.get(&key)?;
        if *expires > now {
            return Some(scope.clone());
        }
        self.entries.remove(&key);
        None
    }

    pub(crate) fn revoke_pane(
        &mut self,
        pane: &PaneId,
    ) {
        self.entries.retain(|_, (scope, _)| scope.pane() != pane);
    }

    pub(crate) fn revoke_host(
        &mut self,
        host: &HostId,
    ) {
        self.retain_hosts(|held| held != host);
    }

    pub(crate) fn retain_hosts(
        &mut self,
        keep: impl Fn(&HostId) -> bool,
    ) {
        self.entries.retain(|_, (scope, _)| keep(scope.host()));
    }
}

fn same_stream(
    a: &RelayScope,
    b: &RelayScope,
) -> bool {
    a.pane() == b.pane() && a.host() == b.host()
}

fn digest(token: &str) -> Digest32 {
    Sha256::digest(token.as_bytes()).into()
}

#[cfg(test)]
mod tests {
    use protocol::PaneOrigin;

    use super::*;

    const TTL: Duration = Duration::from_secs(60);

    fn scope(
        pane: &str,
        host: &str,
    ) -> RelayScope {
        RelayScope::new(
            HostId::new(host).unwrap(),
            PaneId::new(pane).unwrap(),
            PaneOrigin::Core,
        )
    }

    struct Case {
        what: &'static str,
        act: fn(&mut TokenBook),
        later: Duration,
        alive: [bool; 3],
    }

    fn revoke_pane(book: &mut TokenBook) {
        book.revoke_pane(&PaneId::new("p1").unwrap());
    }

    fn revoke_host(book: &mut TokenBook) {
        book.revoke_host(&HostId::new("host-1").unwrap());
    }

    fn retain_local(book: &mut TokenBook) {
        book.retain_hosts(|host| host.as_str() == "local");
    }

    fn issue_again(book: &mut TokenBook) {
        book.issue(scope("p1", "local"), Instant::now()).unwrap();
    }

    fn nothing(_: &mut TokenBook) {}

    #[tokio::test(start_paused = true)]
    async fn a_token_names_its_scope_until_it_expires_is_replaced_or_is_revoked() {
        let cases = [
            Case {
                what: "untouched",
                act: nothing,
                later: Duration::ZERO,
                alive: [true; 3],
            },
            Case {
                what: "expired",
                act: nothing,
                later: TTL + Duration::from_secs(1),
                alive: [false; 3],
            },
            Case {
                what: "issued again for the same stream",
                act: issue_again,
                later: Duration::ZERO,
                alive: [false, true, true],
            },
            Case {
                what: "a pane revoked",
                act: revoke_pane,
                later: Duration::ZERO,
                alive: [false, true, true],
            },
            Case {
                what: "a host revoked",
                act: revoke_host,
                later: Duration::ZERO,
                alive: [true, false, true],
            },
            Case {
                what: "hosts retained",
                act: retain_local,
                later: Duration::ZERO,
                alive: [true, false, true],
            },
        ];
        for case in cases {
            let mut book = TokenBook::new(TTL);
            let scopes = [
                scope("p1", "local"),
                scope("p2", "host-1"),
                scope("p3", "local"),
            ];
            let tokens = scopes
                .clone()
                .map(|scope| book.issue(scope, Instant::now()).unwrap());
            (case.act)(&mut book);
            let now = Instant::now() + case.later;
            let alive = tokens.clone().map(|token| book.authorize(&token, now));
            for ((live, wanted), scope) in alive.iter().zip(case.alive).zip(&scopes) {
                let expected = wanted.then(|| scope.clone());
                assert_eq!(live, &expected, "{}", case.what);
            }
        }
    }

    #[tokio::test(start_paused = true)]
    async fn tokens_are_long_random_and_distinct() {
        let mut book = TokenBook::new(TTL);
        let one = book.issue(scope("p1", "local"), Instant::now()).unwrap();
        let two = book.issue(scope("p2", "local"), Instant::now()).unwrap();
        assert_ne!(one, two);
        assert_eq!(one.len(), 43);
        assert!(
            one.bytes()
                .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
        );
        assert_eq!(book.authorize("not-a-token", Instant::now()), None);
    }
}
