//! Device connections: read from the daemon, and changed only by its answer.

use std::sync::Arc;

use daemonapi::Call;
use serde::Deserialize;
use settings::{Change, DEVICE_ADDRESS, DEVICE_CONNECTIONS, DEVICE_LISTENING, Key, Value};

use crate::live::Outcome;
use crate::store::settings_core::{SettingsCore, locked};

const ACCESS_PATH: &str = "/v0/access";
const FORBIDDEN: u16 = 403;
const ACCESS_VALUES: usize = 3;
const REFUSED_TITLE: &str = "Couldn't change device connections";
// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/hosts/hooks/use-device-access.ts
const NOT_THE_OWNER: &str =
    "Device connections can only be changed on the computer this instance runs on.";

#[derive(Debug, Deserialize)]
struct Access {
    enabled: bool,
    address: String,
    reachable: Vec<String>,
}

impl Access {
    fn listening(&self) -> String {
        if self.reachable.is_empty() {
            return self.address.clone();
        }
        self.reachable.join(", ")
    }

    fn values(&self) -> [(&'static str, Value); ACCESS_VALUES] {
        [
            (DEVICE_CONNECTIONS, Value::Bool(self.enabled)),
            (DEVICE_ADDRESS, Value::Text(self.address.clone())),
            (DEVICE_LISTENING, Value::Text(self.listening())),
        ]
    }
}

impl SettingsCore {
    pub(crate) async fn read_access(&self) {
        match self.local.send(Call::get(ACCESS_PATH)).await {
            Ok(data) => self.adopt_access(data),
            Err(error) => tracing::warn!(%error, "the device connections could not be read"),
        }
    }

    fn adopt_access(
        &self,
        data: serde_json::Value,
    ) {
        let access = match serde_json::from_value::<Access>(data) {
            Ok(access) => access,
            Err(error) => return tracing::warn!(%error, "the device connections do not parse"),
        };
        for (key, value) in access.values() {
            locked(&self.values).insert(key.to_owned(), value.clone());
            self.push(Change::Setting(Key::new(key), value));
        }
    }

    pub(crate) fn send_access(
        self: &Arc<Self>,
        previous: Option<Value>,
    ) {
        let core = Arc::clone(self);
        self.handle
            .spawn(async move { core.put_access(previous).await });
    }

    async fn put_access(
        &self,
        previous: Option<Value>,
    ) {
        let enabled = self.get(&Key::new(DEVICE_CONNECTIONS)) == Some(Value::Bool(true));
        let address = self
            .get(&Key::new(DEVICE_ADDRESS))
            .and_then(|value| value.as_text().map(str::to_owned))
            .unwrap_or_default();
        let body = serde_json::json!({"enabled": enabled, "address": address});
        match self.local.send(Call::put(ACCESS_PATH).json(body)).await {
            Ok(data) => self.adopt_access(data),
            Err(error) => self.refuse_access(&error, previous),
        }
    }

    fn refuse_access(
        &self,
        error: &daemonapi::Error,
        previous: Option<Value>,
    ) {
        tracing::warn!(%error, "the device connections were not changed");
        self.restore(DEVICE_CONNECTIONS, previous.clone());
        self.push(Change::Setting(
            Key::new(DEVICE_CONNECTIONS),
            previous.unwrap_or(Value::Bool(false)),
        ));
        let reason = match error {
            daemonapi::Error::Refused { status, .. } if *status == FORBIDDEN => {
                NOT_THE_OWNER.to_owned()
            },
            daemonapi::Error::Refused { message, .. } => message.clone(),
            other => other.to_string(),
        };
        (self.outcome)(Outcome::Refused {
            title: REFUSED_TITLE.to_owned(),
            reason,
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::SettingsRig;

    const OFF: (&str, &str, &str) = (
        "GET",
        ACCESS_PATH,
        r#"{"enabled":false,"address":"0.0.0.0:3737","reachable":[],"fingerprint":"f"}"#,
    );
    const ON: (&str, &str, &str) = (
        "PUT",
        ACCESS_PATH,
        r#"{"enabled":true,"address":"0.0.0.0:3737","reachable":["192.168.1.4:3737","10.0.0.2:3737"],"fingerprint":"f"}"#,
    );

    #[tokio::test]
    async fn turning_connections_on_sends_the_address_and_shows_where_devices_reach_it() {
        let rig = SettingsRig::open(&[OFF, ON]);
        rig.called(ACCESS_PATH).await;
        let read = Change::Setting(Key::new(DEVICE_ADDRESS), Value::Text("0.0.0.0:3737".into()));
        rig.changes_until(|all| all.contains(&read)).await;
        rig.core
            .set(&Key::new(DEVICE_CONNECTIONS), Value::Bool(true));
        let listening = Change::Setting(
            Key::new(DEVICE_LISTENING),
            Value::Text("192.168.1.4:3737, 10.0.0.2:3737".into()),
        );
        rig.changes_until(|all| all.contains(&listening)).await;
        let put = rig
            .daemon
            .seen()
            .into_iter()
            .find(|seen| seen.method == "PUT");
        let body = put.and_then(|seen| seen.body);
        assert_eq!(
            body,
            Some(serde_json::json!({"enabled": true, "address": "0.0.0.0:3737"}))
        );
    }

    #[tokio::test]
    async fn a_refused_change_moves_the_switch_back_and_says_why_in_develop_s_words() {
        let rig = SettingsRig::open(&[OFF]);
        rig.daemon.answer(
            ("PUT", ACCESS_PATH),
            FORBIDDEN,
            r#"{"success":false,"error":"device connections can only be changed from this computer"}"#,
        );
        let read = Change::Setting(Key::new(DEVICE_ADDRESS), Value::Text("0.0.0.0:3737".into()));
        rig.changes_until(|all| all.contains(&read)).await;
        rig.core
            .set(&Key::new(DEVICE_CONNECTIONS), Value::Bool(true));
        let back = Change::Setting(Key::new(DEVICE_CONNECTIONS), Value::Bool(false));
        rig.changes_until(|all| all.contains(&back)).await;
        assert_eq!(
            rig.core.get(&Key::new(DEVICE_CONNECTIONS)),
            Some(Value::Bool(false))
        );
        assert_eq!(
            *locked(&rig.told),
            [Outcome::Refused {
                title: REFUSED_TITLE.to_owned(),
                reason: NOT_THE_OWNER.to_owned(),
            }]
        );
    }
}
