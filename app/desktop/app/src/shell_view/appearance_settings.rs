use gpui::Context;
use settings_ui::{Change, Key, SettingsBackend, Value};
use ui::{Theme, ThemeId, ThemeMode};

use super::ShellView;
use super::sidebar_position::SIDEBAR_POSITION;
use crate::family_names::FamilyNames;
use crate::shell_request::ShellRequest;
use crate::terminal_preferences::{KEYS, terminal_settings};

const THEME: &str = "theme";
const MODE: &str = "themeMode";
const SIZE: &str = "uiFontSize";
const UI_FAMILY: &str = "uiFontFamily";
const HEADING_FAMILY: &str = "headingFontFamily";
const MONO_FAMILY: &str = "fontFamily";

fn is_appearance(key: &Key) -> bool {
    [
        THEME,
        MODE,
        SIZE,
        UI_FAMILY,
        HEADING_FAMILY,
        MONO_FAMILY,
        SIDEBAR_POSITION,
    ]
    .contains(&key.as_str())
        || KEYS.contains(&key.as_str())
}

fn notify_when_appearance_moves(
    requests: &async_channel::Sender<ShellRequest>,
    change: &Change,
) {
    let moved = matches!(change, Change::Setting(key, _) if is_appearance(key));
    if moved && requests.try_send(ShellRequest::AppearanceChanged).is_err() {
        tracing::debug!("the shell is closing; an appearance change goes unseen");
    }
}

fn id_of(stored: Option<&Value>) -> ThemeId {
    match stored.and_then(Value::as_text) {
        Some("frosted") => ThemeId::Frosted,
        _ => ThemeId::Crowbar,
    }
}

fn mode_of(stored: Option<&Value>) -> ThemeMode {
    match stored.and_then(Value::as_text) {
        Some("light") => ThemeMode::Light,
        Some("dark") => ThemeMode::Dark,
        _ => ThemeMode::System,
    }
}

type Stored<'a> = (
    (Option<&'a Value>, Option<&'a Value>, Option<&'a Value>),
    (Option<&'a Value>, Option<&'a Value>, Option<&'a Value>),
);

fn family_of(
    names: &mut FamilyNames,
    stored: Option<&Value>,
) -> Option<&'static str> {
    stored
        .and_then(Value::as_text)
        .and_then(|name| names.intern(name))
}

#[expect(
    clippy::cast_possible_truncation,
    reason = "a font size in points fits an f32 exactly enough"
)]
pub(super) fn themed(
    base: Theme,
    (stored, names): (Stored<'_>, &mut FamilyNames),
) -> Theme {
    let ((theme, mode, size), (ui, heading, mono)) = stored;
    let themed = base.with_id(id_of(theme)).with_mode(mode_of(mode));
    let sized = size
        .and_then(Value::as_number)
        .map_or(themed, |points| themed.with_ui_font_size(points as f32));
    let faced = family_of(names, ui).map_or(sized, |family| sized.with_ui_family(family));
    let headed =
        family_of(names, heading).map_or(faced, |family| faced.with_heading_family(family));
    family_of(names, mono).map_or(headed, |family| headed.with_mono_family(family))
}

impl ShellView {
    pub(super) fn watch_appearance(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let requests = self.requests.clone();
        self.appearance_watch =
            Some(backend.subscribe(std::rc::Rc::new(move |change: &Change| {
                notify_when_appearance_moves(&requests, change);
            })));
        self.apply_appearance(cx);
    }

    pub(super) fn apply_appearance(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let stored = (
            (
                backend.get(&Key::new(THEME)),
                backend.get(&Key::new(MODE)),
                backend.get(&Key::new(SIZE)),
            ),
            (
                backend.get(&Key::new(UI_FAMILY)),
                backend.get(&Key::new(HEADING_FAMILY)),
                backend.get(&Key::new(MONO_FAMILY)),
            ),
        );
        let ((theme, mode, size), (ui, heading, mono)) = stored;
        let stored = (
            (theme.as_ref(), mode.as_ref(), size.as_ref()),
            (ui.as_ref(), heading.as_ref(), mono.as_ref()),
        );
        let theme = themed(*Theme::of(cx), (stored, &mut self.family_names));
        theme.install(cx);
        let terminal = terminal_settings(|key| backend.get(&Key::new(key)));
        self.source.apply_terminal_settings(terminal, cx);
        self.apply_sidebar_position(cx);
        self.refresh(cx);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stored_values_pick_the_theme_mode_and_scale_and_unset_ones_keep_the_defaults() {
        let base = Theme::default();
        let frosted = Value::Text("frosted".into());
        let dark = Value::Text("dark".into());
        let size = Value::Number(30.0);
        let ui = Value::Text("Inter".into());
        let heading = Value::Text("Lora".into());
        let mono = Value::Text("Fira Code".into());
        let mut names = FamilyNames::default();
        let stored = (
            (Some(&frosted), Some(&dark), Some(&size)),
            (Some(&ui), Some(&heading), Some(&mono)),
        );
        let set = themed(base, (stored, &mut names));
        assert_eq!(
            (set.ui_family(), set.heading_family(), set.mono_family()),
            ("Inter", "Lora", "Fira Code")
        );
        assert_eq!((set.id(), set.mode()), (ThemeId::Frosted, ThemeMode::Dark));
        assert!(set.ui_scale() > base.ui_scale());
        let unset = themed(base, (((None, None, None), (None, None, None)), &mut names));
        assert_eq!(
            (unset.ui_family(), unset.heading_family()),
            (base.ui_family(), base.heading_family())
        );
        assert_eq!(
            (unset.id(), unset.mode()),
            (ThemeId::Crowbar, ThemeMode::System)
        );
    }
}
