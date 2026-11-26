//! The devices paired with this computer's daemon, newest first, each with its revoke.

use std::sync::Arc;

use daemonapi::Call;
use settings::{Change, DEVICE_RECORDS};

use crate::live::Outcome;
use crate::store::settings_core::devices::device::{Device, still_paired};
use crate::store::settings_core::{Loadable, SettingsCore, locked};

const CLIENTS_PATH: &str = "/v0/clients";

impl SettingsCore {
    pub(crate) async fn read_devices(&self) {
        let next = match self.local.read::<Vec<Device>>(CLIENTS_PATH).await {
            Ok(all) => Loadable::Ready(still_paired(all)),
            Err(error) => {
                tracing::warn!(%error, "the paired devices could not be listed");
                Loadable::Failed
            },
        };
        *locked(&self.devices) = next;
        self.push(Change::Records(DEVICE_RECORDS.to_owned()));
    }

    pub(crate) fn reread_devices(self: &Arc<Self>) {
        let core = Arc::clone(self);
        self.handle.spawn(async move { core.read_devices().await });
    }

    pub(crate) fn revoke_device(
        self: &Arc<Self>,
        id: &str,
    ) {
        let path = format!("{CLIENTS_PATH}/{id}");
        self.handle.spawn(Arc::clone(self).revoke_then_list(path));
    }

    async fn revoke_then_list(
        self: Arc<Self>,
        path: String,
    ) {
        if let Err(error) = self.local.send(Call::delete(path)).await {
            self.revoke_failed(error);
        }
        self.read_devices().await;
    }

    fn revoke_failed(
        &self,
        error: daemonapi::Error,
    ) {
        tracing::warn!(%error, "a device could not be revoked");
        (self.outcome)(Outcome::Failed(crate::Error::Daemon(error).reason()));
    }
}

#[cfg(test)]
mod tests {
    use settings::{RecordCommand, Records};

    use super::*;
    use crate::store::settings_core::devices::device::REVOKE_DEVICE;
    use crate::test_support::SettingsRig;

    const CLIENTS: (&str, &str, &str) = (
        "GET",
        CLIENTS_PATH,
        r#"[
            {"id":"old","name":"Old phone","pairedAt":"2026-01-02T12:00:00Z","lastSeen":"0001-01-01T00:00:00Z"},
            {"id":"gone","name":"Gone","pairedAt":"2026-05-01T12:00:00Z","lastSeen":"2026-05-01T12:00:00Z","revokedAt":"2026-06-01T12:00:00Z"},
            {"id":"new","name":"Pixel","pairedAt":"2026-11-20T12:00:00Z","lastSeen":"2026-11-25T12:00:00Z"}
        ]"#,
    );

    fn devices_listed(changes: &[Change]) -> bool {
        changes.contains(&Change::Records(DEVICE_RECORDS.to_owned()))
    }

    fn shown(rig: &SettingsRig) -> Vec<(String, String)> {
        let Records::Ready(records) = rig.core.records(DEVICE_RECORDS) else {
            return Vec::new();
        };
        records
            .iter()
            .map(|record| {
                let line = record.lines().first().map(|line| line.text().to_owned());
                (record.title().to_owned(), line.unwrap_or_default())
            })
            .collect()
    }

    #[tokio::test]
    async fn paired_devices_are_listed_newest_first_without_the_revoked_ones() {
        let rig = SettingsRig::open(&[CLIENTS]);
        rig.changes_until(devices_listed).await;
        assert_eq!(
            shown(&rig),
            [
                (
                    "Pixel".to_owned(),
                    "Paired Nov 20, 2026 \u{b7} Last seen Nov 25, 2026".to_owned()
                ),
                ("Old phone".to_owned(), "Paired Jan 2, 2026".to_owned()),
            ]
        );
    }

    #[tokio::test]
    async fn revoking_a_device_tells_the_daemon_and_lists_the_devices_again() {
        let rig = SettingsRig::open(&[CLIENTS, ("DELETE", "/v0/clients/new", "{}")]);
        rig.changes_until(devices_listed).await;
        let run = RecordCommand::Run {
            action: REVOKE_DEVICE.to_owned(),
        };
        assert!(rig.core.apply(DEVICE_RECORDS, "new", run).is_ok());
        rig.called("/v0/clients/new").await;
        rig.changes_until(devices_listed).await;
        let lists = rig
            .daemon
            .seen()
            .iter()
            .filter(|seen| seen.path == CLIENTS_PATH)
            .count();
        assert_eq!(lists, 2, "listed on open and again after the revoke");
    }
}
