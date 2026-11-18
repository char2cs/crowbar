//! Removing a paired host: its pairing is revoked on the host first, because forgetting this
//! computer's key first would drop the only credential that can.

use std::sync::Arc;

use daemonapi::{Call, DaemonClient};
use tokio::runtime::Handle;

use crate::error::Error;
use crate::live::fleet::Fleet;
use crate::live::fleet::host_desk::HostDesk;
use crate::live::outcome::{Outcome, OutcomeSink};
use crate::live::state::ChangeSignal;

const REFUSED_BY_HOST: u16 = 401;
const NO_CLIENT_ID: &str =
    "This computer has no record of its pairing with the host, so it cannot revoke it there.";
const STILL_LISTED: &str = "It will still list this computer as paired.";

pub(crate) struct HostRoster {
    fleet: Arc<Fleet>,
    desk: HostDesk,
    outcome: OutcomeSink,
    signal: ChangeSignal,
}

impl HostRoster {
    pub(crate) fn new(
        (fleet, desk): (Arc<Fleet>, HostDesk),
        (outcome, signal): (OutcomeSink, ChangeSignal),
    ) -> Arc<Self> {
        Arc::new(Self {
            fleet,
            desk,
            outcome,
            signal,
        })
    }

    pub(crate) fn revoke(
        self: &Arc<Self>,
        handle: &Handle,
        host: &str,
    ) {
        let roster = Arc::clone(self);
        let host = host.to_owned();
        handle.spawn(async move { roster.revoke_now(&host).await });
    }

    pub(crate) fn remove_anyway(
        self: &Arc<Self>,
        handle: &Handle,
        host: &str,
    ) {
        let roster = Arc::clone(self);
        let host = host.to_owned();
        handle.spawn(async move { roster.forget(&host).await });
    }

    fn client_of(
        &self,
        host: &str,
    ) -> Option<DaemonClient> {
        self.fleet.hub().read(|state| {
            state
                .hosts
                .iter()
                .find(|slot| slot.spec.id == host && slot.spec.endpoint.is_some())
                .map(|slot| slot.client.clone())
        })
    }

    async fn revoke_now(
        &self,
        host: &str,
    ) {
        let (Some(client), Some(note)) = (self.client_of(host), self.desk.notes.get(host)) else {
            return self.keep(host, NO_CLIENT_ID);
        };
        let path = format!("/v0/clients/{}", note.client_id);
        match client.send(Call::delete(path)).await {
            Ok(_) => self.forget(host).await,
            Err(daemonapi::Error::Refused { status, .. }) if status == REFUSED_BY_HOST => {
                self.forget(host).await;
            },
            Err(error) => self.keep(host, &Error::from(error).reason()),
        }
    }

    fn keep(
        &self,
        host: &str,
        why: &str,
    ) {
        let message = format!("Couldn’t revoke it on the host: {why} {STILL_LISTED}");
        self.fleet.hub().edit(|state| {
            state.unrevoked.insert(host.to_owned(), message.clone());
        });
        (self.outcome)(Outcome::Failed(message));
        (self.signal)();
    }

    async fn forget(
        &self,
        host: &str,
    ) {
        self.fleet.forget(host);
        if let Err(error) = self.desk.book.unpair(host).await {
            tracing::warn!(%error, host, "the key of a removed host could not be forgotten");
            (self.outcome)(Outcome::Failed(error.to_string()));
        }
        if let Err(error) = self.desk.notes.remove(host) {
            tracing::warn!(%error, host, "the note of a removed host could not be dropped");
        }
        (self.signal)();
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use super::*;
    use crate::live::fleet::host_desk::host_note::HostNote;
    use crate::live::held::HostSpec;
    use crate::live::state::Hub;
    use crate::test_support::{CLIENT, FakeBook, FakeDaemon, notes, state};

    struct Rig {
        roster: Arc<HostRoster>,
        book: Arc<FakeBook>,
        daemon: Arc<FakeDaemon>,
        hub: Arc<Hub>,
        told: Arc<Mutex<Vec<Outcome>>>,
        fleet: Arc<Fleet>,
    }

    fn desk_in(book: &Arc<FakeBook>) -> HostDesk {
        let notes = Arc::new(notes());
        let note = HostNote {
            name: "Lab".into(),
            client_id: CLIENT.into(),
            fingerprint: "f".into(),
        };
        notes.put("lab", note).unwrap();
        HostDesk {
            book: Arc::<FakeBook>::clone(book),
            notes,
        }
    }

    fn rig() -> Rig {
        let book = Arc::new(FakeBook::default());
        let (state, daemon) = state();
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let hub = Hub::new(state, Arc::new(|| {}));
        let fleet = Fleet::new(&Handle::current(), &hub, &transport);
        let desk = desk_in(&book);
        let spec = HostSpec {
            id: "lab".into(),
            name: "Lab".into(),
            endpoint: Some("box:3737".into()),
        };
        fleet.follow(spec).unwrap();
        let told = Arc::new(Mutex::new(Vec::new()));
        let sink = Arc::clone(&told);
        let outcome: OutcomeSink = Arc::new(move |outcome| sink.lock().unwrap().push(outcome));
        let roster = HostRoster::new((Arc::clone(&fleet), desk), (outcome, Arc::new(|| {})));
        Rig {
            roster,
            book,
            daemon,
            hub,
            told,
            fleet,
        }
    }

    fn listed(rig: &Rig) -> usize {
        rig.hub.read(|state| state.hosts.len())
    }

    const DELETE: &str = "/v0/clients/client-1";

    async fn a_host_that_revokes_is_forgotten_everywhere() {
        let rig = rig();
        rig.daemon.ok("DELETE", DELETE, "null");
        rig.roster.revoke_now("lab").await;
        assert_eq!(listed(&rig), 1);
        assert_eq!(rig.fleet.len(), 0);
        assert_eq!(rig.book.unpaired(), ["lab"]);
        assert_eq!(rig.roster.desk.notes.get("lab"), None);
        assert!(rig.daemon.seen().iter().any(|seen| seen.method == "DELETE"));
    }

    async fn a_host_that_refuses_this_client_holds_nothing_to_revoke() {
        let rig = rig();
        rig.daemon.answer(
            ("DELETE", DELETE),
            401,
            r#"{"success":false,"error":"unknown client"}"#,
        );
        rig.roster.revoke_now("lab").await;
        assert_eq!(listed(&rig), 1);
        assert!(rig.told.lock().unwrap().is_empty());
    }

    async fn a_failed_revoke_keeps_the_host_until_it_is_removed_anyway() {
        let rig = rig();
        rig.daemon.answer(
            ("DELETE", DELETE),
            500,
            r#"{"success":false,"error":"disk"}"#,
        );
        rig.roster.revoke_now("lab").await;
        assert_eq!(listed(&rig), 2);
        assert!(rig.hub.read(|state| state.unrevoked.contains_key("lab")));
        assert!(
            matches!(&rig.told.lock().unwrap()[..], [Outcome::Failed(text)] if text.contains("still list"))
        );
        rig.roster.forget("lab").await;
        assert_eq!(listed(&rig), 1);
        assert!(rig.hub.read(|state| state.unrevoked.is_empty()));
    }

    type Case = fn() -> std::pin::Pin<Box<dyn Future<Output = ()>>>;

    const CASES: [Case; 3] = [
        || Box::pin(a_host_that_revokes_is_forgotten_everywhere()),
        || Box::pin(a_host_that_refuses_this_client_holds_nothing_to_revoke()),
        || Box::pin(a_failed_revoke_keeps_the_host_until_it_is_removed_anyway()),
    ];

    async fn run_all() {
        for case in CASES {
            case().await;
        }
    }

    #[tokio::test]
    async fn every_case_ends_as_it_says() {
        run_all().await;
    }
}
