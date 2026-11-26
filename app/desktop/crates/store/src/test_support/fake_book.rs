use std::sync::Mutex;

use crate::live::BookError;
use crate::live::PairingRequest;
use crate::live::{BookFuture, HostBook};

/// A [`HostBook`] with a fixed fingerprint and client id that records what it was asked.
#[derive(Default)]
pub(crate) struct FakeBook {
    probe_error: Mutex<Option<String>>,
    pair_error: Mutex<Option<String>>,
    paired: Mutex<Vec<PairingRequest>>,
    unpaired: Mutex<Vec<String>>,
}

pub(crate) const PRINT: &str = "AA:BB";
pub(crate) const CLIENT: &str = "client-1";

fn held<T>(mutex: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    mutex
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
}

impl FakeBook {
    pub(crate) fn failing_probe(text: &str) -> Self {
        let book = Self::default();
        *held(&book.probe_error) = Some(text.to_owned());
        book
    }

    pub(crate) fn failing_pair(text: &str) -> Self {
        let book = Self::default();
        *held(&book.pair_error) = Some(text.to_owned());
        book
    }

    pub(crate) fn paired(&self) -> Vec<PairingRequest> {
        held(&self.paired).clone()
    }

    pub(crate) fn unpaired(&self) -> Vec<String> {
        held(&self.unpaired).clone()
    }
}

fn answer<T: Send + 'static>(
    failure: Option<String>,
    value: T,
) -> BookFuture<T> {
    Box::pin(std::future::ready(failure.map_or(Ok(value), |text| {
        Err(BookError::new(std::io::Error::other(text)))
    })))
}

impl HostBook for FakeBook {
    fn probe(
        &self,
        _endpoint: &str,
    ) -> BookFuture<String> {
        answer(held(&self.probe_error).clone(), PRINT.to_owned())
    }

    fn pair(
        &self,
        request: PairingRequest,
    ) -> BookFuture<String> {
        held(&self.paired).push(request);
        answer(held(&self.pair_error).clone(), CLIENT.to_owned())
    }

    fn unpair(
        &self,
        host: &str,
    ) -> BookFuture<()> {
        held(&self.unpaired).push(host.to_owned());
        answer(None, ())
    }
}

/// A desk over a fake book and in-memory notes.
pub(crate) fn desk() -> crate::live::HostDesk {
    crate::live::HostDesk {
        book: std::sync::Arc::new(FakeBook::default()),
        notes: std::sync::Arc::new(super::notes()),
    }
}

/// A fleet over `hub` and a desk, for tests that open a settings core.
pub(crate) fn fleet_and_desk(
    hub: &std::sync::Arc<crate::live::Hub>
) -> (std::sync::Arc<crate::live::Fleet>, crate::live::HostDesk) {
    let transport: std::sync::Arc<dyn protocol::DaemonTransport> = super::FakeDaemon::shared();
    let fleet = crate::live::Fleet::new(
        &tokio::runtime::Handle::current(),
        hub,
        (&transport, std::sync::Arc::new(|_| {})),
    );
    (fleet, desk())
}
