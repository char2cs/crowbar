use std::sync::Arc;

use tauri::http::HeaderValue;

use crate::api_proxy::remote::{bearer, stored_credential, RemoteHost};
use crate::host_tokens::TokenStore;
use crate::hosts::HostRegistry;

pub(super) struct PairedHost {
    pub(super) host_id: String,
    pub(super) host: RemoteHost,
    pub(super) authorization: HeaderValue,
}

pub(super) async fn paired_host(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
) -> Result<PairedHost, String> {
    let endpoint = registry
        .get(host_id)
        .ok_or_else(|| format!("unknown remote host {host_id}"))?;
    let credential = stored_credential(tokens, host_id)
        .await
        .map_err(|e| format!("token lookup failed: {e}"))?
        .ok_or("no stored credential for this host — pair it again")?;
    Ok(PairedHost {
        host_id: host_id.to_owned(),
        authorization: bearer(&credential.token)?,
        host: RemoteHost::paired(endpoint, &credential),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::proxy_fakes::{paired, refused_endpoint, registry, Keychain, HOST_ID};

    const NONE: &str = "no stored credential";

    type Scenario = (Arc<dyn TokenStore>, &'static str);

    fn keychains() -> Vec<Scenario> {
        vec![
            (Keychain::holding(&[]), NONE),
            (
                Keychain::holding(&[("another-host", &paired("t", b""))]),
                NONE,
            ),
            (Keychain::holding(&[(HOST_ID, &paired("", b""))]), NONE),
            (Keychain::holding(&[(HOST_ID, &paired(" \t", b""))]), NONE),
            (Keychain::holding(&[(HOST_ID, "sometoken")]), NONE),
            (
                Keychain::holding(&[(HOST_ID, &paired("some\ntoken", b""))]),
                "not a valid header value",
            ),
            (Keychain::broken("keychain is locked"), "keychain is locked"),
        ]
    }

    #[tokio::test]
    async fn a_host_without_a_usable_credential_is_refused_before_dialing() {
        a_host_without_a_usable_credential_is_re_run().await;
    }

    async fn a_host_without_a_usable_credential_is_re_run() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(&refused_endpoint());
        for (case, (keychain, reason)) in keychains().into_iter().enumerate() {
            let outcome = paired_host(&registry, &keychain, HOST_ID).await;
            let err = outcome.err().unwrap_or_default();
            assert!(err.contains(reason), "case {case}: {err}");
        }
    }

    #[tokio::test]
    async fn an_unregistered_host_is_unknown_and_a_registered_one_is_paired() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(&refused_endpoint());
        let keychain = Keychain::holding(&[(HOST_ID, &paired("sometoken", b"cert"))]);
        let pair = paired_host(&registry, &keychain, HOST_ID).await;
        assert_eq!(pair.map(|p| p.host_id), Ok(HOST_ID.to_owned()));
        registry.remove(HOST_ID);
        let gone = paired_host(&registry, &keychain, HOST_ID).await;
        let error = gone.err().unwrap_or_default();
        assert!(error.contains("unknown remote host"), "{error}");
    }
}
