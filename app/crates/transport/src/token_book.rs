use std::collections::HashMap;

use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use protocol::{HostId, PaneId, RelayScope};
use sha2::{Digest, Sha256};
use tokio::time::{Duration, Instant};

use crate::error::Error;

const TOKEN_BYTES: usize = 32;

type Digest32 = [u8; 32];

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

    #[tokio::test(start_paused = true)]
    async fn a_token_names_its_scope_until_it_expires() {
        let mut book = TokenBook::new(Duration::from_secs(60));
        let token = book.issue(scope("p1", "local"), Instant::now()).unwrap();
        assert_eq!(
            book.authorize(&token, Instant::now()),
            Some(scope("p1", "local"))
        );
        tokio::time::advance(Duration::from_secs(61)).await;
        assert_eq!(book.authorize(&token, Instant::now()), None);
    }

    #[tokio::test(start_paused = true)]
    async fn tokens_are_long_random_and_distinct() {
        let mut book = TokenBook::new(Duration::from_secs(60));
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

    #[tokio::test(start_paused = true)]
    async fn issuing_again_for_the_same_stream_replaces_the_old_token() {
        let mut book = TokenBook::new(Duration::from_secs(60));
        let old = book.issue(scope("p1", "local"), Instant::now()).unwrap();
        let other_host = book.issue(scope("p1", "host-1"), Instant::now()).unwrap();
        let fresh = book.issue(scope("p1", "local"), Instant::now()).unwrap();
        assert_eq!(book.authorize(&old, Instant::now()), None);
        assert!(book.authorize(&fresh, Instant::now()).is_some());
        assert!(book.authorize(&other_host, Instant::now()).is_some());
    }

    #[tokio::test(start_paused = true)]
    async fn revoking_a_pane_or_a_host_drops_exactly_their_tokens() {
        let mut book = TokenBook::new(Duration::from_secs(60));
        let a = book.issue(scope("p1", "local"), Instant::now()).unwrap();
        let b = book.issue(scope("p2", "host-1"), Instant::now()).unwrap();
        let c = book.issue(scope("p3", "local"), Instant::now()).unwrap();
        book.revoke_pane(&PaneId::new("p1").unwrap());
        book.revoke_host(&HostId::new("host-1").unwrap());
        assert_eq!(book.authorize(&a, Instant::now()), None);
        assert_eq!(book.authorize(&b, Instant::now()), None);
        assert!(book.authorize(&c, Instant::now()).is_some());
    }

    #[tokio::test(start_paused = true)]
    async fn retaining_hosts_drops_the_tokens_of_hosts_that_are_no_longer_kept() {
        let mut book = TokenBook::new(Duration::from_secs(60));
        let kept = book.issue(scope("p1", "local"), Instant::now()).unwrap();
        let gone = book.issue(scope("p2", "host-1"), Instant::now()).unwrap();
        book.retain_hosts(|host| host.as_str() == "local");
        assert!(book.authorize(&kept, Instant::now()).is_some());
        assert_eq!(book.authorize(&gone, Instant::now()), None);
    }
}
