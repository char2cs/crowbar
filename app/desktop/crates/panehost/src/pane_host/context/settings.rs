use protocol::{
    PANE_SETTING_KEYS, PaneFrame, PaneId, SETTINGS_FRAME_VERSION, SettingValues, ShellFrame,
};

use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::slot::HostEvent;

impl PaneHost {
    /// Replaces the settings every pane follows and pushes the whole set to mounted panes; a
    /// pane mounted later gets it on mount. An unchanged set sends nothing.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_settings(
        &mut self,
        values: SettingValues,
    ) -> Result<()> {
        if self.settings.as_ref() == Some(&values) {
            return Ok(());
        }
        let frame = settings_frame(values.clone());
        self.settings = Some(values);
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    pub(crate) fn settings_context(&self) -> Option<ShellFrame> {
        self.settings.clone().map(settings_frame)
    }
}

fn settings_frame(values: SettingValues) -> ShellFrame {
    ShellFrame::Settings {
        version: SETTINGS_FRAME_VERSION,
        values,
    }
}

pub(crate) fn setting_event(
    pane: PaneId,
    frame: PaneFrame,
) -> Vec<HostEvent> {
    let PaneFrame::SettingsSet {
        version,
        key,
        value,
    } = frame
    else {
        return Vec::new();
    };
    if version == SETTINGS_FRAME_VERSION && PANE_SETTING_KEYS.contains(&key.as_str()) {
        return vec![HostEvent::SettingRequested { pane, key, value }];
    }
    tracing::warn!(%pane, %key, version, "a pane wrote a setting the shell does not share");
    Vec::new()
}

#[cfg(test)]
mod tests {
    use protocol::SettingKey;
    use serde_json::{Value, json};

    use super::*;
    use crate::slot::IpcMessage;
    use crate::test_support::*;

    fn wrap(value: Value) -> SettingValues {
        SettingValues::default().with("wordWrap", value).unwrap()
    }

    fn settings_sent(f: &Fixture) -> Vec<SettingValues> {
        frames_sent(&f.mock)
            .into_iter()
            .filter_map(|frame| match frame {
                ShellFrame::Settings { version, values } => {
                    assert_eq!(version, SETTINGS_FRAME_VERSION, "version");
                    Some(values)
                },
                _ => None,
            })
            .collect()
    }

    fn mounted() -> Fixture {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        f
    }

    fn write(
        version: u32,
        key: &str,
    ) -> PaneFrame {
        PaneFrame::SettingsSet {
            version,
            key: SettingKey::new(key).unwrap(),
            value: json!(true),
        }
    }

    #[test]
    fn a_pane_gets_the_settings_on_mount_and_on_every_change() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f.host.set_settings(wrap(json!(false))).unwrap();
        assert!(settings_sent(&f).is_empty(), "not mounted yet");
        register(&mut f, "p1", "core.chat");
        f.host.set_settings(wrap(json!(true))).unwrap();
        f.host.set_settings(wrap(json!(true))).unwrap();
        assert_eq!(
            settings_sent(&f),
            vec![wrap(json!(false)), wrap(json!(true))],
            "on mount, then once per change"
        );
    }

    #[test]
    fn nothing_is_sent_before_the_shell_has_settings() {
        assert!(settings_sent(&mounted()).is_empty(), "no settings known");
    }

    #[test]
    fn a_pane_write_reaches_the_shell_only_for_a_shared_key_of_a_known_version() {
        let pane = PaneId::new("p1").unwrap();
        let cases = [
            (write(SETTINGS_FRAME_VERSION, "wordWrap"), 1),
            (write(SETTINGS_FRAME_VERSION, "theme"), 0),
            (write(SETTINGS_FRAME_VERSION, "sidebarPosition"), 0),
            (write(SETTINGS_FRAME_VERSION + 1, "wordWrap"), 0),
        ];
        for (frame, events) in cases {
            let name = format!("{frame:?}");
            assert_eq!(setting_event(pane.clone(), frame).len(), events, "{name}");
        }
    }

    #[test]
    fn a_settings_set_frame_from_a_page_becomes_a_host_event() {
        let mut f = mounted();
        let pane = PaneId::new("p1").unwrap();
        let text = protocol::encode(&write(SETTINGS_FRAME_VERSION, "wordWrap")).unwrap();
        let events = f.ipc(IpcMessage {
            pane: pane.clone(),
            text,
        });
        assert_eq!(
            events,
            vec![HostEvent::SettingRequested {
                pane,
                key: SettingKey::new("wordWrap").unwrap(),
                value: json!(true),
            }]
        );
    }
}
