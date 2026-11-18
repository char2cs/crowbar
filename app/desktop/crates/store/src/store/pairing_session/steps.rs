//! The steps of pairing that wait on the host: probing its certificate and redeeming the code.

use crate::live::HostSpec;
use crate::live::Note;
use crate::live::PairingRequest;
use crate::store::pairing_session::PairingSession;
use crate::store::pairing_session::pairing_pending::PairingPending;
use crate::store::pairing_session::pairing_stage::PairingStage;
use crate::store::pairing_session::words::Words;
use crate::store::pairing_session::words::{already_paired, failure as pairing_failure};

impl PairingSession {
    pub(crate) async fn probe(
        &self,
        name: String,
        endpoint: String,
    ) {
        let probed = self.desk.book.probe(&endpoint).await;
        let outcome = probed
            .map_err(|error| {
                Words::new(pairing_failure::words(&error.to_string(), &endpoint, &name))
            })
            .and_then(|fingerprint| self.unpaired(&endpoint, fingerprint));
        self.update(|held| match outcome {
            Ok(fingerprint) => held.confirm_next(&name, &endpoint, fingerprint),
            Err(words) => held.state.error = Some(words.into_text()),
        });
        self.update(|held| held.state.pending = PairingPending::Idle);
    }

    fn unpaired(
        &self,
        endpoint: &str,
        fingerprint: String,
    ) -> Result<String, Words> {
        let known = already_paired::reason(
            (self.fleet.hub(), &self.desk.notes),
            (endpoint, &fingerprint),
        );
        known.map_or(Ok(fingerprint), |reason| Err(Words::new(reason)))
    }

    pub(crate) async fn redeem(
        &self,
        request: PairingRequest,
    ) {
        let paired = self.desk.book.pair(request.clone()).await;
        let outcome = match paired {
            Ok(client_id) => self.keep(&request, client_id).await,
            Err(error) => Err(Words::new(pairing_failure::words(
                &error.to_string(),
                &request.endpoint,
                &request.name,
            ))),
        };
        self.update(|held| match outcome {
            Ok(()) => held.state.stage = PairingStage::Paired,
            Err(words) => held.state.error = Some(words.into_text()),
        });
        self.update(|held| held.state.pending = PairingPending::Idle);
    }

    /// Remembers the pairing and starts following the host; a pairing that cannot be remembered
    /// is undone, because a host without its client id could never be revoked.
    async fn keep(
        &self,
        request: &PairingRequest,
        client_id: String,
    ) -> Result<(), Words> {
        let note = Note {
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
        Err(Words::new(pairing_failure::words(
            "",
            &request.endpoint,
            &request.name,
        )))
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::live::HostBook;
    use crate::live::HostDesk;
    use crate::test_support::{CLIENT, FakeBook, PRINT, notes, state};

    fn session_over(book: FakeBook) -> (Arc<PairingSession>, Arc<FakeBook>) {
        let (state, daemon) = state();
        let transport: Arc<dyn protocol::DaemonTransport> = daemon;
        let hub = crate::live::Hub::new(state, Arc::new(|| {}));
        let fleet = crate::live::Fleet::new(&tokio::runtime::Handle::current(), &hub, &transport);
        let book = Arc::new(book);
        let desk = HostDesk {
            book: Arc::<FakeBook>::clone(&book) as Arc<dyn HostBook>,
            notes: Arc::new(notes()),
        };
        let session = PairingSession::new((fleet, desk), Arc::new(|| {}));
        (session, book)
    }

    fn request() -> PairingRequest {
        PairingRequest {
            host_id: "lab-1".into(),
            endpoint: "lab.local:3737".into(),
            fingerprint: PRINT.into(),
            code: "123456".into(),
            name: "Lab".into(),
        }
    }

    #[tokio::test]
    async fn a_probe_confirms_the_fingerprint_it_read_and_a_redeem_follows_the_host() {
        let (session, book) = session_over(FakeBook::default());
        session.probe("Lab".into(), "lab.local:3737".into()).await;
        assert_eq!(session.state().fingerprint, PRINT);
        assert_eq!(session.state().stage, PairingStage::Fingerprint);
        session.redeem(request()).await;
        assert_eq!(session.state().stage, PairingStage::Paired);
        assert_eq!(book.paired().len(), 1);
        let note = session.desk.notes.get("lab-1").unwrap();
        assert_eq!(note.client_id, CLIENT);
        assert_eq!(session.fleet.len(), 1);
    }

    #[tokio::test]
    async fn a_refused_probe_or_pairing_says_why_in_plain_words() {
        let (session, _) = session_over(FakeBook::failing_probe("cannot reach lab.local:3737"));
        session.probe("Lab".into(), "lab.local:3737".into()).await;
        assert!(session.state().error.unwrap().contains("Can’t reach"));
        let refusal = "the host refused pairing (401 Unauthorized): no";
        let (session, _) = session_over(FakeBook::failing_pair(refusal));
        session.redeem(request()).await;
        assert!(
            session
                .state()
                .error
                .unwrap()
                .contains("That code didn’t work")
        );
        assert_eq!(session.fleet.len(), 0);
    }
}
