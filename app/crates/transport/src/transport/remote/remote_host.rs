use crate::hosts::Endpoint;
use crate::transport::remote::secrets::Credential;
use crate::transport::remote::tls::Pin;

#[derive(Clone, Debug)]
pub(crate) struct RemoteHost {
    pub(crate) endpoint: Endpoint,
    pub(crate) pin: Pin,
}

impl RemoteHost {
    pub(crate) fn paired(
        endpoint: Endpoint,
        credential: &Credential,
    ) -> Self {
        Self {
            endpoint,
            pin: Pin::Paired(credential.pin()),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::transport::remote::tls::certificate_digest;

    #[test]
    fn a_paired_host_is_pinned_to_the_digest_stored_with_its_credential() {
        let pin = certificate_digest(b"cert");
        let credential = Credential::new("t".into(), pin);
        let host = RemoteHost::paired(Endpoint::parse("10.0.0.2:7000").unwrap(), &credential);
        assert_eq!(host.pin, Pin::Paired(pin));
    }
}
