use crate::host_tokens::Credential;
use crate::hosts::endpoint::Endpoint;
use crate::tls_pin::Pin;

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
            pin: Pin::Paired(credential.pin),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hosts::endpoint::parse_endpoint;

    #[test]
    fn a_paired_host_is_pinned_to_the_certificate_its_credential_was_minted_by() {
        let endpoint = parse_endpoint("1.2.3.4:3737").unwrap();
        let credential = Credential {
            token: "t".into(),
            pin: std::array::from_fn(|i| u8::try_from(i).unwrap()),
        };

        let host = RemoteHost::paired(endpoint.clone(), &credential);

        assert_eq!(host.endpoint, endpoint);
        assert!(matches!(host.pin, Pin::Paired(pin) if pin == credential.pin));
    }
}
