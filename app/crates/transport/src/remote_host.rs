use crate::credential::Credential;
use crate::endpoint::Endpoint;
use crate::pin::Pin;

/// Where a remote daemon listens, and the one certificate trusted there.
#[derive(Clone, Debug)]
pub struct RemoteHost {
    /// The address dialled.
    pub endpoint: Endpoint,
    /// The only certificate the dial accepts.
    pub pin: Pin,
}

impl RemoteHost {
    /// Pinned to the certificate `credential` was minted by, never to anything the registry, and
    /// so the webview, supplies.
    #[must_use]
    pub fn paired(endpoint: Endpoint, credential: &Credential) -> Self {
        Self {
            endpoint,
            pin: Pin::Paired(credential.pin),
        }
    }
}

#[cfg(test)]
#[path = "remote_host_tests.rs"]
mod tests;
