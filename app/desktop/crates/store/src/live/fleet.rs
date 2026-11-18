//! The hosts being followed: a stream per host, started and stopped while the shell runs.

pub(crate) mod catalog;
mod host_desk;
mod host_dialling;
mod host_stream;
mod member;
mod stream_control;

pub(crate) use catalog::{Provider, Surface};
pub(crate) use host_desk::HostRoster;
pub use host_desk::{
    BookError, BookFuture, HostBook, HostDesk, HostNote, HostNotes, PairingRequest,
};
pub(crate) use host_dialling::{Dialled, relay_for, slot_for};

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use daemonapi::{DaemonClient, Dial};
use protocol::DaemonTransport;
use tokio::runtime::Handle;
use tokio::sync::mpsc;

use crate::error::{Error, Result};
use crate::live::fleet::host_stream::HostStream;
use crate::live::fleet::member::Member;
use crate::live::fleet::stream_control::StreamControl;
use crate::live::held::HostSpec;
use crate::live::state::Hub;

pub(crate) struct Fleet {
    handle: Handle,
    hub: Arc<Hub>,
    transport: Arc<dyn DaemonTransport>,
    members: Mutex<BTreeMap<String, Member>>,
}

impl std::fmt::Debug for Fleet {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Fleet")
            .field("streams", &self.len())
            .finish_non_exhaustive()
    }
}

impl Fleet {
    pub(crate) fn new(
        handle: &Handle,
        hub: &Arc<Hub>,
        transport: &Arc<dyn DaemonTransport>,
    ) -> Arc<Self> {
        Arc::new(Self {
            handle: handle.clone(),
            hub: Arc::clone(hub),
            transport: Arc::clone(transport),
            members: Mutex::default(),
        })
    }

    pub(crate) fn hub(&self) -> &Arc<Hub> {
        &self.hub
    }

    pub(crate) fn handle(&self) -> &Handle {
        &self.handle
    }

    fn held(&self) -> MutexGuard<'_, BTreeMap<String, Member>> {
        self.members.lock().unwrap_or_else(PoisonError::into_inner)
    }

    pub(crate) fn len(&self) -> usize {
        self.held().len()
    }

    pub(crate) fn run(
        &self,
        (host, client): (String, DaemonClient),
        dial: Arc<dyn Dial>,
    ) {
        let (control, inbox) = mpsc::unbounded_channel();
        let stream = HostStream {
            hub: Arc::clone(&self.hub),
            host: host.clone(),
            dial,
            client,
        };
        let task = self.handle.spawn(stream.run(inbox));
        if let Some(replaced) = self.held().insert(host, Member { control, task }) {
            replaced.task.abort();
        }
    }

    pub(crate) fn follow(
        &self,
        spec: HostSpec,
    ) -> Result<()> {
        if self.held().contains_key(&spec.id) {
            return Err(Error::Stream(format!("{} is already followed", spec.id)));
        }
        let (spec, dial) = relay_for(&self.transport, spec)
            .ok_or_else(|| Error::Stream("the host id cannot be addressed".to_owned()))?;
        let slot = slot_for(&self.transport, spec)
            .ok_or_else(|| Error::Stream("the host id cannot be addressed".to_owned()))?;
        let followed = (slot.spec.id.clone(), slot.client.clone());
        self.hub.edit(|state| state.add_host(slot));
        self.run(followed, dial);
        Ok(())
    }

    pub(crate) fn forget(
        &self,
        host: &str,
    ) {
        if let Some(member) = self.held().remove(host) {
            member.task.abort();
        }
        self.hub.edit(|state| state.remove_host(host));
    }

    pub(crate) fn resubscribe(
        &self,
        host: Option<&str>,
    ) {
        self.held()
            .iter()
            .filter(|(id, _)| host.is_none_or(|wanted| wanted == id.as_str()))
            .for_each(|(id, member)| resubscribe(id, member));
    }

    pub(crate) fn stop_all(&self) {
        self.held().values().for_each(|member| member.task.abort());
    }
}

fn resubscribe(
    id: &str,
    member: &Member,
) {
    if member.control.send(StreamControl::Resubscribe).is_err() {
        tracing::warn!(host = %id, "the stream task is gone; the chats cannot be reloaded");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    fn fleet() -> (Arc<Fleet>, Arc<Hub>) {
        let (state, daemon) = state();
        let transport: Arc<dyn DaemonTransport> = daemon;
        let hub = Hub::new(state, Arc::new(|| {}));
        (Fleet::new(&Handle::current(), &hub, &transport), hub)
    }

    fn spec(id: &str) -> HostSpec {
        HostSpec {
            id: id.to_owned(),
            name: id.to_owned(),
            endpoint: Some("box:3737".to_owned()),
        }
    }

    fn hosts_of(hub: &Hub) -> Vec<String> {
        hub.read(|state| {
            state
                .hosts
                .iter()
                .map(|slot| slot.spec.id.clone())
                .collect()
        })
    }

    #[tokio::test]
    async fn a_followed_host_joins_the_hub_once_and_a_forgotten_one_leaves_it() {
        let (fleet, hub) = fleet();
        fleet.follow(spec("lab")).unwrap();
        assert_eq!(hosts_of(&hub), ["local", "lab"]);
        assert_eq!(fleet.len(), 1);
        assert!(fleet.follow(spec("lab")).is_err(), "twice is refused");
        assert!(fleet.follow(spec("Bad Id")).is_err(), "an unaddressable id");
        fleet.forget("lab");
        assert_eq!(hosts_of(&hub), ["local"]);
        assert_eq!(fleet.len(), 0);
    }
}
