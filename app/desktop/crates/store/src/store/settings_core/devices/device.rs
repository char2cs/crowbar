//! A device paired with this computer's daemon, and how its row on the Devices page reads.

use serde::Deserialize;
use settings::{Confirm, Line, Record, RecordAction};

use crate::store::pairing::local_dates::day;

pub(crate) const REVOKE_DEVICE: &str = "revoke";
const ICON: &str = "device-mobile";

#[derive(Debug, Clone, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct Device {
    id: String,
    name: String,
    paired_at: String,
    last_seen: String,
    #[serde(default)]
    revoked_at: Option<String>,
}

fn detail(device: &Device) -> String {
    let paired = format!("Paired {}", day(&device.paired_at).unwrap_or_default());
    match day(&device.last_seen) {
        Some(seen) => format!("{paired} \u{b7} Last seen {seen}"),
        None => paired,
    }
}

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/settings/components/tabs/devices-settings.tsx
fn revoke(device: &Device) -> RecordAction {
    let confirm = Confirm::new(
        format!("Revoke {}?", device.name),
        format!(
            "{} will be disconnected and will need a new code to connect again.",
            device.name
        ),
        "Revoke",
    );
    RecordAction::new(REVOKE_DEVICE, "trash", format!("Revoke {}", device.name)).confirm(confirm)
}

pub(crate) fn records(devices: &[Device]) -> Vec<Record> {
    devices
        .iter()
        .map(|device| {
            Record::new(device.id.as_str(), device.name.as_str())
                .icon(ICON)
                .line(Line::plain(detail(device)))
                .action(revoke(device))
        })
        .collect()
}

pub(crate) fn still_paired(mut all: Vec<Device>) -> Vec<Device> {
    all.retain(|device| device.revoked_at.is_none());
    all.sort_by(|a, b| b.paired_at.cmp(&a.paired_at));
    all
}

#[cfg(test)]
mod tests {
    use super::*;

    fn device(
        id: &str,
        paired_at: &str,
        revoked_at: Option<&str>,
    ) -> Device {
        Device {
            id: id.to_owned(),
            name: id.to_owned(),
            paired_at: paired_at.to_owned(),
            last_seen: "0001-01-01T00:00:00Z".to_owned(),
            revoked_at: revoked_at.map(str::to_owned),
        }
    }

    #[test]
    fn a_device_never_seen_reads_only_when_it_paired() {
        let records = records(&[device("Pixel", "2026-11-20T12:00:00Z", None)]);
        let lines: Vec<&str> = records[0].lines().iter().map(Line::text).collect();
        assert_eq!(lines, ["Paired Nov 20, 2026"]);
    }
}
