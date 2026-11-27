use std::sync::{Arc, Mutex, PoisonError};

use daemonapi::Call;
use tokio::runtime::Handle;

use crate::live::ChangeSignal;
use crate::store::pairing::offer::Offer;
use crate::store::pairing::offer_state::OfferState;

const CODE_PATH: &str = "/v0/pair/code";

pub struct DeviceOffer {
    client: daemonapi::Client,
    handle: Handle,
    signal: ChangeSignal,
    state: Mutex<OfferState>,
}

impl std::fmt::Debug for DeviceOffer {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("DeviceOffer")
            .finish_non_exhaustive()
    }
}

impl DeviceOffer {
    pub(crate) fn start(
        client: daemonapi::Client,
        handle: Handle,
        signal: ChangeSignal,
    ) -> Arc<Self> {
        let offer = Arc::new(Self {
            client,
            handle,
            signal,
            state: Mutex::new(OfferState::Minting),
        });
        offer.renew();
        offer
    }

    pub fn state(&self) -> OfferState {
        self.state
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clone()
    }

    pub fn renew(self: &Arc<Self>) {
        self.settle(OfferState::Minting);
        let offer = Arc::clone(self);
        self.handle.spawn(async move { offer.mint().await });
    }

    async fn mint(&self) {
        let next = match self.client.fetch::<Offer>(Call::post(CODE_PATH)).await {
            Ok(minted) => OfferState::Ready(minted.shown()),
            Err(error) => {
                tracing::warn!(%error, "no pairing code could be made");
                OfferState::Failed
            },
        };
        self.settle(next);
    }

    fn settle(
        &self,
        next: OfferState,
    ) {
        *self.state.lock().unwrap_or_else(PoisonError::into_inner) = next;
        (self.signal)();
    }
}

#[cfg(test)]
mod tests {
    use tokio::sync::Notify;

    use super::*;
    use crate::store::pairing::local_dates;
    use crate::test_support::{FakeDaemon, local};

    const MINTED: &str = r#"{"code":"123456","serverFingerprint":"SHA256:ab","addresses":["192.168.1.4:3737"],"expiresAt":"2026-11-26T01:15:00Z"}"#;

    fn offer_on(daemon: &Arc<FakeDaemon>) -> (Arc<DeviceOffer>, Arc<Notify>) {
        let client = daemonapi::Client::new(Arc::<FakeDaemon>::clone(daemon), local());
        let changed = Arc::new(Notify::new());
        let notify = Arc::clone(&changed);
        let signal: ChangeSignal = Arc::new(move || notify.notify_one());
        (
            DeviceOffer::start(client, Handle::current(), signal),
            changed,
        )
    }

    async fn settled(
        offer: &DeviceOffer,
        changed: &Notify,
    ) -> OfferState {
        while offer.state() == OfferState::Minting {
            changed.notified().await;
        }
        offer.state()
    }

    #[tokio::test]
    async fn a_minted_code_is_grouped_with_where_to_reach_this_computer() {
        let daemon = FakeDaemon::shared();
        daemon.ok("POST", CODE_PATH, MINTED);
        let (offer, changed) = offer_on(&daemon);
        let expected = Offer {
            code: "123 456".to_owned(),
            until: local_dates::time("2026-11-26T01:15:00Z").unwrap_or_default(),
            addresses: vec!["192.168.1.4:3737".to_owned()],
            fingerprint: "SHA256:ab".to_owned(),
        };
        assert_eq!(settled(&offer, &changed).await, OfferState::Ready(expected));
    }

    #[tokio::test]
    async fn a_refused_mint_fails_and_a_new_code_asks_again() {
        let daemon = FakeDaemon::shared();
        daemon.answer(
            ("POST", CODE_PATH),
            409,
            r#"{"success":false,"error":"off"}"#,
        );
        let (offer, changed) = offer_on(&daemon);
        assert_eq!(settled(&offer, &changed).await, OfferState::Failed);
        daemon.ok("POST", CODE_PATH, MINTED);
        offer.renew();
        assert!(matches!(
            settled(&offer, &changed).await,
            OfferState::Ready(_)
        ));
        let mints = daemon
            .seen()
            .iter()
            .filter(|seen| seen.path == CODE_PATH)
            .count();
        assert_eq!(mints, 2);
    }
}
