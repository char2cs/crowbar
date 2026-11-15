use std::sync::Mutex;

use crate::host_book::{BookFuture, HostBook};
use crate::pairing_request::PairingRequest;

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
    Box::pin(std::future::ready(failure.map_or(Ok(value), Err)))
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

/// A desk over a fake book and notes in a temporary directory, which lives as long as it does.
pub(crate) fn desk() -> (crate::host_desk::HostDesk, tempfile::TempDir) {
    let dir = tempfile::tempdir().unwrap();
    let notes = crate::host_notes::HostNotes::open(dir.path().join("paired.json")).unwrap();
    let desk = crate::host_desk::HostDesk {
        book: std::sync::Arc::new(FakeBook::default()),
        notes: std::sync::Arc::new(notes),
    };
    (desk, dir)
}

/// A fleet over `hub` and a desk, for tests that open a settings core.
pub(crate) fn fleet_and_desk(
    hub: &std::sync::Arc<crate::hub::Hub>
) -> (
    std::sync::Arc<crate::fleet::Fleet>,
    crate::host_desk::HostDesk,
) {
    let transport: std::sync::Arc<dyn protocol::DaemonTransport> = super::FakeDaemon::shared();
    let fleet = crate::fleet::Fleet::new(&tokio::runtime::Handle::current(), hub, &transport);
    let (desk, dir) = desk();
    drop(dir.keep());
    (fleet, desk)
}
