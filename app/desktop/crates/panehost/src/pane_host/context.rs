//! What panes are told: the frames every mounted page follows, which a page mounted later gets on
//! mount, the settings pages write, and the background web views show.

use std::mem::discriminant;

use protocol::{
    Breakpoint, KeyBinding, PANE_SETTING_KEYS, PairedHost, PaneFrame, PaneId,
    SETTINGS_FRAME_VERSION, SettingValues, ShellFrame, ThemeTokens,
};
use webview::Color;

use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::slot::HostEvent;

const UNZOOMED: u16 = 100;

impl PaneHost {
    /// Replaces the active keybindings and pushes them to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_keybindings(
        &mut self,
        bindings: Vec<KeyBinding>,
    ) -> Result<()> {
        self.set_context(ShellFrame::Keybindings { bindings })
    }

    /// Replaces the theme tokens and pushes them to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_theme(
        &mut self,
        tokens: ThemeTokens,
    ) -> Result<()> {
        self.set_context(ShellFrame::Theme { tokens })
    }

    /// Replaces the breakpoints the window clears and pushes them to mounted panes, so a resize
    /// between thresholds stays quiet.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_breakpoints(
        &mut self,
        cleared: Vec<Breakpoint>,
    ) -> Result<()> {
        self.set_context(ShellFrame::Breakpoints { cleared })
    }

    /// Replaces the window's chat zoom, in percent, and pushes it to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_chat_zoom(
        &mut self,
        percent: u16,
    ) -> Result<()> {
        self.set_context(ShellFrame::ChatZoom { percent })
    }

    /// The window's chat zoom in percent: 100 until the first change.
    #[must_use]
    pub fn chat_zoom(&self) -> u16 {
        self.context
            .iter()
            .find_map(|frame| match frame {
                ShellFrame::ChatZoom { percent } => Some(*percent),
                _ => None,
            })
            .unwrap_or(UNZOOMED)
    }

    /// Replaces the settings every pane follows and pushes the whole set to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_settings(
        &mut self,
        values: SettingValues,
    ) -> Result<()> {
        self.set_context(ShellFrame::Settings {
            version: SETTINGS_FRAME_VERSION,
            values,
        })
    }

    /// Replaces the hosts the shell paired and pushes the list to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_paired_hosts(
        &mut self,
        hosts: Vec<PairedHost>,
    ) -> Result<()> {
        self.set_context(ShellFrame::Hosts { hosts })
    }

    /// Sets the colour pane web views show before their page paints: on every live view now and
    /// on every view created later. Browser views keep the engine default so a page without a
    /// background of its own stays white.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a view cannot be reached.
    pub fn set_background(
        &mut self,
        color: Color,
    ) -> Result<()> {
        self.factory.set_background(color);
        self.pool
            .views()
            .filter(|(kind, _)| **kind != webview::Kind::Browser)
            .try_for_each(|(_, handle)| handle.set_background(color))?;
        Ok(())
    }

    fn set_context(
        &mut self,
        frame: ShellFrame,
    ) -> Result<()> {
        if self.context.contains(&frame) {
            return Ok(());
        }
        let kind = discriminant(&frame);
        self.context.retain(|known| discriminant(known) != kind);
        let sent = self
            .panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame));
        self.context.push(frame);
        sent
    }

    pub(crate) fn send_context(
        &self,
        pane: &PaneId,
    ) -> Result<()> {
        self.context
            .iter()
            .filter(|frame| !matches!(frame, ShellFrame::Keybindings { bindings } if bindings.is_empty()))
            .try_for_each(|frame| self.send(pane, frame))
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
    use protocol::{CommandId, DisplayText, HostId, KeyChord, SettingKey};
    use serde_json::json;
    use webview::Call;

    use super::*;
    use crate::slot::IpcMessage;
    use crate::test_support::*;

    type Set = fn(&mut PaneHost, u8) -> Result<()>;
    type Frame = fn(u8) -> ShellFrame;

    fn binding(nth: u8) -> Vec<KeyBinding> {
        let (keys, command) = [("mod+k", "palette.open"), ("mod+j", "tabs.new")][usize::from(nth)];
        let chord = KeyChord::new(keys).unwrap();
        vec![KeyBinding::new(chord, CommandId::new(command).unwrap())]
    }

    fn theme(nth: u8) -> ThemeTokens {
        let colour = ["#fff", "#000"][usize::from(nth)];
        ThemeTokens::default().with("color-bg", colour).unwrap()
    }

    fn breakpoints(nth: u8) -> Vec<Breakpoint> {
        let mut cleared = vec![Breakpoint::Sm];
        cleared.extend((nth == 1).then_some(Breakpoint::Lg));
        cleared
    }

    fn zoom(nth: u8) -> u16 {
        [110, 120][usize::from(nth)]
    }

    fn settings(nth: u8) -> SettingValues {
        SettingValues::default()
            .with("wordWrap", json!(nth == 1))
            .unwrap()
    }

    fn hosts(nth: u8) -> Vec<PairedHost> {
        let host = PairedHost {
            id: HostId::new("h1").unwrap(),
            name: DisplayText::new("h1").unwrap(),
            endpoint: DisplayText::new("127.0.0.1:4747").unwrap(),
        };
        (nth == 0).then_some(host).into_iter().collect()
    }

    const CASES: &[(&str, Set, Frame)] = &[
        (
            "keybindings",
            |h, b| h.set_keybindings(binding(b)),
            |b| ShellFrame::Keybindings {
                bindings: binding(b),
            },
        ),
        (
            "theme",
            |h, b| h.set_theme(theme(b)),
            |b| ShellFrame::Theme { tokens: theme(b) },
        ),
        (
            "breakpoints",
            |h, b| h.set_breakpoints(breakpoints(b)),
            |b| ShellFrame::Breakpoints {
                cleared: breakpoints(b),
            },
        ),
        (
            "chat zoom",
            |h, b| h.set_chat_zoom(zoom(b)),
            |b| ShellFrame::ChatZoom { percent: zoom(b) },
        ),
        (
            "settings",
            |h, b| h.set_settings(settings(b)),
            |b| ShellFrame::Settings {
                version: SETTINGS_FRAME_VERSION,
                values: settings(b),
            },
        ),
        (
            "paired hosts",
            |h, b| h.set_paired_hosts(hosts(b)),
            |b| ShellFrame::Hosts { hosts: hosts(b) },
        ),
    ];

    #[test]
    fn a_pane_gets_each_context_frame_on_mount_and_once_per_change() {
        for (name, set, frame) in CASES {
            let mut f = fixture();
            f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
            set(&mut f.host, 0).unwrap();
            assert!(frames_sent(&f.mock).is_empty(), "{name}: not mounted yet");
            register(&mut f, "p1", "core.chat");
            set(&mut f.host, 0).unwrap();
            set(&mut f.host, 1).unwrap();
            let kind = discriminant(&frame(0));
            let sent: Vec<ShellFrame> = frames_sent(&f.mock)
                .into_iter()
                .filter(|sent| discriminant(sent) == kind)
                .collect();
            assert_eq!(
                sent,
                [frame(0), frame(1)],
                "{name}: on mount, then on change"
            );
        }
    }

    #[test]
    fn a_pane_mounted_before_any_context_hears_none_and_the_chat_is_unzoomed() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        let told = frames_sent(&f.mock).into_iter().filter(|frame| {
            !matches!(
                frame,
                ShellFrame::Lifecycle { .. } | ShellFrame::Group { .. }
            )
        });
        assert_eq!(told.count(), 0, "no context yet");
        assert_eq!(f.host.chat_zoom(), 100, "unzoomed");
        f.host.set_chat_zoom(120).unwrap();
        assert_eq!(f.host.chat_zoom(), 120, "changed");
    }

    #[test]
    fn a_pane_write_reaches_the_shell_only_for_a_shared_key_of_a_known_version() {
        let cases = [
            (SETTINGS_FRAME_VERSION, "wordWrap", true),
            (SETTINGS_FRAME_VERSION, "theme", false),
            (SETTINGS_FRAME_VERSION, "sidebarPosition", false),
            (SETTINGS_FRAME_VERSION + 1, "wordWrap", false),
        ];
        for (version, key, heard) in cases {
            let key = SettingKey::new(key).unwrap();
            let expected = requested(key.clone());
            let events = written(version, key);
            assert_eq!(
                events,
                Vec::from_iter(heard.then_some(expected)),
                "{version}"
            );
        }
    }

    fn requested(key: SettingKey) -> HostEvent {
        let (pane, value) = (PaneId::new("p1").unwrap(), json!(true));
        HostEvent::SettingRequested { pane, key, value }
    }

    fn written(
        version: u32,
        key: SettingKey,
    ) -> Vec<HostEvent> {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        let value = json!(true);
        let frame = PaneFrame::SettingsSet {
            version,
            key,
            value,
        };
        let text = protocol::encode(&frame).unwrap();
        let pane = PaneId::new("p1").unwrap();
        f.ipc(IpcMessage { pane, text })
    }

    #[test]
    fn a_theme_change_recolours_the_views_that_already_exist() {
        const SURFACE: Color = Color {
            red: 250,
            green: 250,
            blue: 250,
            alpha: 255,
        };
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f.host.set_background(SURFACE).unwrap();
        let recoloured = f.mock.calls().into_iter().filter_map(|call| match call {
            Call::SetBackground { color, .. } => Some(color),
            _ => None,
        });
        assert_eq!(
            recoloured.collect::<Vec<_>>(),
            [SURFACE],
            "the one live view"
        );
    }
}
