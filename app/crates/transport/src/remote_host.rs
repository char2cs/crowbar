use crate::credential::Credential;
use crate::endpoint::Endpoint;
use crate::pin::Pin;

#[derive(Clone, Debug)]
pub(crate) struct RemoteHost {
    pub(crate) endpoint: Endpoint,
    pub(crate) pin: Pin,
}

impl RemoteHost {
    pub(crate) fn paired(endpoint: Endpoint, credential: &Credential) -> Self {
        Self {
            endpoint,
            pin: Pin::Paired(credential.pin()),
        }
    }
}

#[cfg(test)]
#[path = "remote_host_tests.rs"]
mod tests;
