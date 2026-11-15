use gpui::Context;
use settings_ui::{Change, Key, SettingsBackend, Value};
use ui::{Theme, ThemeId, ThemeMode};

use super::ShellView;
use crate::shell_request::ShellRequest;

const THEME: &str = "theme";
const MODE: &str = "themeMode";
const SIZE: &str = "uiFontSize";

fn is_appearance(key: &Key) -> bool {
    [THEME, MODE, SIZE].contains(&key.as_str())
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

type Stored<'a> = (Option<&'a Value>, Option<&'a Value>, Option<&'a Value>);

#[expect(
    clippy::cast_possible_truncation,
    reason = "a font size in points fits an f32 exactly enough"
)]
pub(super) fn themed(
    base: Theme,
    stored: Stored<'_>,
) -> Theme {
    let (theme, mode, size) = stored;
    let themed = base.with_id(id_of(theme)).with_mode(mode_of(mode));
    size.and_then(Value::as_number)
        .map_or(themed, |points| themed.with_ui_font_size(points as f32))
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
            backend.get(&Key::new(THEME)),
            backend.get(&Key::new(MODE)),
            backend.get(&Key::new(SIZE)),
        );
        let theme = themed(
            *Theme::of(cx),
            (stored.0.as_ref(), stored.1.as_ref(), stored.2.as_ref()),
        );
        theme.install(cx);
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
        let set = themed(base, (Some(&frosted), Some(&dark), Some(&size)));
        assert_eq!((set.id(), set.mode()), (ThemeId::Frosted, ThemeMode::Dark));
        assert!(set.ui_scale() > base.ui_scale());
        let unset = themed(base, (None, None, None));
        assert_eq!(
            (unset.id(), unset.mode()),
            (ThemeId::Crowbar, ThemeMode::System)
        );
    }
}
