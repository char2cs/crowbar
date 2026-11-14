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

    pub(crate) fn issue(&mut self, scope: RelayScope, now: Instant) -> Result<String, Error> {
        let mut raw = [0u8; TOKEN_BYTES];
        getrandom::fill(&mut raw).map_err(|e| Error::Random(e.to_string()))?;
        let token = URL_SAFE_NO_PAD.encode(raw);
        self.entries
            .retain(|_, (held, expires)| *expires > now && !same_stream(held, &scope));
        self.entries.insert(digest(&token), (scope, now + self.ttl));
        Ok(token)
    }

    pub(crate) fn authorize(&mut self, token: &str, now: Instant) -> Option<RelayScope> {
        let key = digest(token);
        let (scope, expires) = self.entries.get(&key)?;
        if *expires > now {
            return Some(scope.clone());
        }
        self.entries.remove(&key);
        None
    }

    pub(crate) fn revoke_pane(&mut self, pane: &PaneId) {
        self.entries.retain(|_, (scope, _)| scope.pane != *pane);
    }

    pub(crate) fn revoke_host(&mut self, host: &HostId) {
        self.entries.retain(|_, (scope, _)| scope.host != *host);
    }
}

fn same_stream(a: &RelayScope, b: &RelayScope) -> bool {
    a.pane == b.pane && a.host == b.host
}

fn digest(token: &str) -> Digest32 {
    Sha256::digest(token.as_bytes()).into()
}

#[cfg(test)]
#[path = "token_book_tests.rs"]
mod tests;
