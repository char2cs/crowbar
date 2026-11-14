//! The data behind a [`crate::FakeBackend`].

use std::collections::BTreeMap;

use crate::record_command::RecordCommand;
use crate::records::Records;
use crate::settings_backend::Listener;
use crate::snapshot::Snapshot;

#[derive(Default)]
pub(crate) struct FakeState {
    pub(crate) snapshot: Snapshot,
    pub(crate) records: BTreeMap<String, Records>,
    pub(crate) invoked: Vec<String>,
    pub(crate) commands: Vec<(String, String, RecordCommand)>,
    pub(crate) refusing: bool,
    pub(crate) listeners: BTreeMap<u64, Listener>,
    pub(crate) next_listener: u64,
}
