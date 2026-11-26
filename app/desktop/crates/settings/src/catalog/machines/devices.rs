//! The Devices page: the phones and computers this instance lets connect to it.

use crate::schema::Action;
use crate::schema::ButtonSpec;
use crate::schema::Condition;
use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::Heading;
use crate::schema::ListBlock;
use crate::schema::Page;
use crate::schema::Row;
use crate::schema::Section;
use crate::values::Value;

pub(in crate::catalog) const DEVICES: &str = "devices";
pub(in crate::catalog) const ALLOW: &str = "deviceConnections";
pub(in crate::catalog) const ADDRESS: &str = "deviceListenAddress";
pub(in crate::catalog) const LISTENING: &str = "deviceListening";
const ADD: &str = "devices.add";

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/settings/components/tabs/devices-settings.tsx
pub(in crate::catalog) fn page() -> Page {
    Page::new(DEVICES, "Devices", "device-mobile")
        .block(Heading::new(
            "Devices",
            "Let a phone or another computer connect to this instance. Each device pairs once with a code and can be revoked here.",
        ))
        .block(connections())
        .block(
            ListBlock::new(DEVICES, "No devices paired yet.")
                .titled("Paired devices")
                .states("Loading devices…", "Couldn't load the paired devices."),
        )
        .block(Section::new("Pairing").headerless().row(pairing()))
}

fn allowed() -> Condition {
    Condition::equals(ALLOW, Value::Bool(true))
}

fn refused() -> Condition {
    Condition::equals(ALLOW, Value::Bool(false))
}

fn connections() -> Section {
    let address = Control::text("0.0.0.0:3737", ControlWidth::TextWide);
    Section::new("Device connections")
        .row(
            Row::new(ALLOW, "Allow device connections", Control::switch())
                .describe("Devices with a code can connect while this is on.")
                .describe_while((refused(), "Off. Nothing listens until you turn this on."))
                .default_value(Value::Bool(false))
                .without_reset(),
        )
        .row(Row::new(LISTENING, "Listening on", Control::Readout { mono: true }).when(allowed()))
        .row(
            Row::new(ADDRESS, "Listen address", address)
                .describe("Address and port. 0.0.0.0 listens on every network this computer is on.")
                .describe_while((allowed(), "Turn connections off to change it."))
                .frozen_while(allowed())
                .without_reset(),
        )
}

fn pairing() -> Row {
    let add = ButtonSpec::new("Add a device", Action::backend(ADD))
        .icon("plus")
        .enabled_when(allowed())
        .disabled_note("Turn on device connections first.");
    Row::new("devicePairing", "Pairing", Control::Button(add))
        .describe("Shows a one-time code to enter on the device.")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_device_can_be_added_only_while_connections_are_allowed() {
        let Control::Button(add) = pairing().control().clone() else {
            panic!("the pairing row holds a button");
        };
        assert_eq!(add.enabled, Some(allowed()));
        assert!(add.disabled_note.is_some());
    }
}
