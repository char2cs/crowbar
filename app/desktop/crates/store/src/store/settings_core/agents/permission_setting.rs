use std::sync::Arc;

use daemonapi::Call;
use settings::{Change, Key, Value};

use crate::store::settings_core::agents::permission_key::{PERMISSION, PERMISSION_PATH};
use crate::store::settings_core::{SettingsCore, locked};

impl SettingsCore {
    pub(crate) fn restore(
        &self,
        key: &str,
        previous: Option<Value>,
    ) {
        let mut values = locked(&self.values);
        match previous {
            Some(value) => values.insert(key.to_owned(), value),
            None => values.remove(key),
        };
    }

    pub(crate) fn send_permission(
        self: &Arc<Self>,
        previous: Option<Value>,
        level: String,
    ) {
        let core = Arc::clone(self);
        self.handle
            .spawn(async move { core.put_permission(level, previous).await });
    }

    async fn put_permission(
        &self,
        level: String,
        previous: Option<Value>,
    ) {
        let call = Call::put(PERMISSION_PATH).json(serde_json::json!({"level": level}));
        let Err(error) = self.local.send(call).await else {
            return;
        };
        tracing::warn!(%error, "the default permission level was not saved");
        self.restore(PERMISSION, previous.clone());
        if let Some(old) = previous {
            self.push(Change::Setting(Key::new(PERMISSION), old));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{AGENTS, LEVEL_GUARDED, SettingsRig};

    #[tokio::test]
    async fn the_permission_level_reverts_when_the_daemon_refuses_it() {
        let rig = SettingsRig::open(&[LEVEL_GUARDED, AGENTS]);
        rig.agents_ready().await;
        let key = Key::new(PERMISSION);
        rig.core.set(&key, Value::Text("full-auto".into()));
        let reverted = Change::Setting(key.clone(), Value::Text("guarded".into()));
        let changes = rig.changes_until(|all| all.contains(&reverted)).await;
        assert!(changes.contains(&reverted));
        assert_eq!(rig.core.get(&key), Some(Value::Text("guarded".into())));
    }
}
