//! Adding a host: probe its certificate, have the user confirm the fingerprint, then redeem a
//! code from its `crowbar pair`. The code stage is reached only by that confirm and pins exactly
//! the fingerprint shown, never a re-probed one.

use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use tokio::runtime::Handle;

use crate::already_paired;
use crate::change_signal::ChangeSignal;
use crate::endpoint_text;
use crate::fleet::Fleet;
use crate::host_desk::HostDesk;
use crate::host_note::HostNote;
use crate::host_spec::HostSpec;
use crate::hub::Hub;
use crate::pairing_failure;
use crate::pairing_held::PairingHeld;
use crate::pairing_pending::PairingPending;
use crate::pairing_request::PairingRequest;
use crate::pairing_stage::PairingStage;
use crate::pairing_state::PairingState;

const CODE_DIGITS: usize = 6;
const CODE_SHAPE_NOTICE: &str = "A pairing code is exactly 6 digits.";

/// One attempt at adding a host. Mount one per attempt: a confirmed fingerprint never carries
/// over to the next.
pub struct PairingSession {
    desk: HostDesk,
    fleet: Arc<Fleet>,
    hub: Arc<Hub>,
    handle: Handle,
    signal: ChangeSignal,
    held: Mutex<PairingHeld>,
}

impl std::fmt::Debug for PairingSession {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("PairingSession")
            .finish_non_exhaustive()
    }
}

fn code_shaped(code: &str) -> bool {
    code.len() == CODE_DIGITS && code.bytes().all(|byte| byte.is_ascii_digit())
}

impl PairingSession {
    pub(crate) fn new(
        (hub, fleet, desk): (Arc<Hub>, Arc<Fleet>, HostDesk),
        handle: Handle,
        signal: ChangeSignal,
    ) -> Arc<Self> {
        Arc::new(Self {
            desk,
            fleet,
            hub,
            handle,
            signal,
            held: Mutex::default(),
        })
    }

    fn held(&self) -> MutexGuard<'_, PairingHeld> {
        self.held.lock().unwrap_or_else(PoisonError::into_inner)
    }

    fn update<R>(
        &self,
        change: impl FnOnce(&mut PairingHeld) -> R,
    ) -> R {
        let result = change(&mut self.held());
        (self.signal)();
        result
    }

    /// What the dialog shows now.
    pub fn state(&self) -> PairingState {
        self.held().state.clone()
    }

    /// The typed name or address changed: a failure about the old one is stale.
    pub fn edited(&self) {
        self.update(|held| {
            held.state.error = None;
            held.state.notice = None;
        });
    }

    /// Reads the certificate of the host at `address`, to be named `name`.
    pub fn submit_address(
        self: &Arc<Self>,
        name: &str,
        address: &str,
    ) {
        let name = name.trim().to_owned();
        if name.is_empty() || address.trim().is_empty() {
            return;
        }
        let endpoint = match endpoint_text::normalize(address) {
            Ok(endpoint) => endpoint,
            Err(reason) => return self.fail_with(reason.to_owned()),
        };
        if !self.begin(PairingPending::Probing) {
            return;
        }
        let session = Arc::clone(self);
        self.handle
            .spawn(async move { session.probe(name, endpoint).await });
    }

    fn begin(
        &self,
        pending: PairingPending,
    ) -> bool {
        self.update(|held| held.begin(pending))
    }

    fn fail_with(
        &self,
        error: String,
    ) {
        self.update(|held| held.state.error = Some(error));
    }

    async fn probe(
        &self,
        name: String,
        endpoint: String,
    ) {
        let probed = self.desk.book.probe(&endpoint).await;
        let outcome = probed
            .map_err(|text| pairing_failure::words(&text, &endpoint, &name))
            .and_then(|fingerprint| self.unpaired(&endpoint, fingerprint));
        self.update(|held| match outcome {
            Ok(fingerprint) => held.confirm_next(&name, &endpoint, fingerprint),
            Err(error) => held.state.error = Some(error),
        });
        self.update(|held| held.state.pending = PairingPending::Idle);
    }

    fn unpaired(
        &self,
        endpoint: &str,
        fingerprint: String,
    ) -> Result<String, String> {
        let known = already_paired::reason((&self.hub, &self.desk.notes), (endpoint, &fingerprint));
        known.map_or(Ok(fingerprint), Err)
    }

    /// The user compared the fingerprint with the host's own and accepts it.
    pub fn confirm(&self) {
        self.update(PairingHeld::enter_code);
    }

    /// One stage back, unless the host is answering.
    pub fn back(&self) {
        self.update(|held| {
            let idle = held.state.pending == PairingPending::Idle;
            held.state.error = None;
            held.state.notice = None;
            held.state.stage = match held.state.stage {
                PairingStage::Code if idle => PairingStage::Fingerprint,
                PairingStage::Fingerprint if idle => PairingStage::Address,
                other => other,
            };
        });
    }

    /// Redeems `code` at the host the confirmed fingerprint belongs to.
    pub fn submit_code(
        self: &Arc<Self>,
        code: &str,
    ) {
        let request = {
            let held = self.held();
            let ready = held.state.stage == PairingStage::Code;
            ready.then(|| held.request(code))
        };
        let Some(request) = request else {
            return;
        };
        if !code_shaped(code) {
            return self.update(|held| held.state.notice = Some(CODE_SHAPE_NOTICE.to_owned()));
        }
        if !self.begin(PairingPending::Pairing) {
            return;
        }
        self.update(|held| held.state.notice = None);
        let session = Arc::clone(self);
        self.handle
            .spawn(async move { session.redeem(request).await });
    }

    async fn redeem(
        &self,
        request: PairingRequest,
    ) {
        let paired = self.desk.book.pair(request.clone()).await;
        let outcome = match paired {
            Ok(client_id) => self.keep(&request, client_id).await,
            Err(text) => Err(pairing_failure::words(
                &text,
                &request.endpoint,
                &request.name,
            )),
        };
        self.update(|held| match outcome {
            Ok(()) => held.state.stage = PairingStage::Paired,
            Err(error) => held.state.error = Some(error),
        });
        self.update(|held| held.state.pending = PairingPending::Idle);
    }

    /// Remembers the pairing and starts following the host; a pairing that cannot be remembered
    /// is undone, because a host without its client id could never be revoked.
    async fn keep(
        &self,
        request: &PairingRequest,
        client_id: String,
    ) -> Result<(), String> {
        let note = HostNote {
            name: request.name.clone(),
            client_id,
            fingerprint: request.fingerprint.clone(),
        };
        let spec = HostSpec {
            id: request.host_id.clone(),
            name: request.name.clone(),
            endpoint: Some(request.endpoint.clone()),
        };
        let saved = self.desk.notes.put(&request.host_id, note);
        let followed = saved
            .map_err(|error| error.to_string())
            .and_then(|()| self.fleet.follow(spec).map_err(|error| error.to_string()));
        let Err(text) = followed else {
            return Ok(());
        };
        tracing::warn!(error = %text, host = %request.host_id, "a paired host could not be kept");
        if let Err(error) = self.desk.book.unpair(&request.host_id).await {
            tracing::warn!(%error, "the pairing could not be undone");
        }
        Err(pairing_failure::words("", &request.endpoint, &request.name))
    }
}

#[cfg(test)]
mod tests {
    use tokio::sync::Notify;

    use super::*;
    use crate::host_book::HostBook;
    use crate::host_notes::HostNotes;
    use crate::test_support::{CLIENT, FakeBook, PRINT, state};

    struct Rig {
        session: Arc<PairingSession>,
        hub: Arc<Hub>,
        book: Arc<FakeBook>,
        fleet: Arc<Fleet>,
        changed: Arc<Notify>,
        _dir: tempfile::TempDir,
    }

    fn rig(book: FakeBook) -> Rig {
        let (state, daemon) = state();
        let transport: Arc<dyn protocol::DaemonTransport> = daemon;
        let hub = Hub::new(state, Arc::new(|| {}));
        let fleet = Fleet::new(&Handle::current(), &hub, &transport);
        let dir = tempfile::tempdir().unwrap();
        let notes = HostNotes::open(dir.path().join("paired.json")).unwrap();
        let book = Arc::new(book);
        let desk = HostDesk {
            book: Arc::<FakeBook>::clone(&book) as Arc<dyn HostBook>,
            notes: Arc::new(notes),
        };
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let signal: ChangeSignal = Arc::new(move || wake.notify_one());
        let session = PairingSession::new(
            (Arc::clone(&hub), Arc::clone(&fleet), desk),
            Handle::current(),
            signal,
        );
        Rig {
            session,
            hub,
            book,
            fleet,
            changed,
            _dir: dir,
        }
    }

    async fn settled(rig: &Rig) -> PairingState {
        while rig.session.state().pending != PairingPending::Idle {
            rig.changed.notified().await;
        }
        rig.session.state()
    }

    #[tokio::test]
    async fn a_host_is_paired_against_the_fingerprint_that_was_confirmed() {
        let rig = rig(FakeBook::default());
        rig.session.submit_address("Home Lab", " Lab.Local ");
        let probed = settled(&rig).await;
        assert_eq!(probed.stage, PairingStage::Fingerprint);
        assert_eq!(probed.fingerprint, PRINT);
        rig.session.submit_code("123456");
        assert_eq!(
            rig.session.state().stage,
            PairingStage::Fingerprint,
            "not confirmed yet"
        );
        rig.session.confirm();
        rig.session.submit_code("12");
        assert!(
            rig.session.state().notice.is_some(),
            "a short code is not sent"
        );
        rig.session.submit_code("123456");
        assert_eq!(settled(&rig).await.stage, PairingStage::Paired);
        let sent = rig.book.paired();
        assert_eq!(sent.len(), 1);
        assert_eq!(sent[0].endpoint, "lab.local:3737");
        assert_eq!(sent[0].fingerprint, PRINT);
        assert!(sent[0].host_id.starts_with("home-lab-"));
        assert_eq!(rig.fleet.len(), 1);
        assert_eq!(rig.hub.read(|state| state.hosts.len()), 2);
        let note = rig.session.desk.notes.get(&sent[0].host_id).unwrap();
        assert_eq!(
            (note.name.as_str(), note.client_id.as_str()),
            ("Home Lab", CLIENT)
        );
    }

    async fn probed_with(book: FakeBook) -> (Rig, PairingState) {
        let rig = rig(book);
        rig.session.submit_address("Lab", "lab.local");
        let state = settled(&rig).await;
        (rig, state)
    }

    async fn paired_with(book: FakeBook) -> (Rig, PairingState) {
        let (rig, _) = probed_with(book).await;
        rig.session.confirm();
        rig.session.submit_code("123456");
        let state = settled(&rig).await;
        (rig, state)
    }

    fn assert_refused(
        (rig, state): &(Rig, PairingState),
        words: &str,
    ) {
        let error = state.error.clone().unwrap_or_default();
        assert!(error.contains(words), "{error}");
        assert_ne!(state.stage, PairingStage::Paired);
        assert_eq!(rig.fleet.len(), 0);
    }

    #[tokio::test]
    async fn a_failed_step_says_why_and_leaves_nothing_followed() {
        let unreachable = FakeBook::failing_probe("cannot reach lab.local:3737");
        assert_refused(&probed_with(unreachable).await, "Can’t reach");
        let wrong = FakeBook::failing_pair("the host refused pairing (401 Unauthorized): no");
        assert_refused(&paired_with(wrong).await, "That code didn’t work");
    }
}
