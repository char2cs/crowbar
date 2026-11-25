use gpui::Context;
use settings::{Key, Value, Write as _};
use ui::ToastKind;

use crate::shell::shell_view::ShellView;
use crate::shell::window::notice_key::NoticeKey;

fn refusal(error: &settings::Error) -> (ToastKind, String) {
    (
        ToastKind::Error,
        format!("The setting was not saved: {error}"),
    )
}

impl ShellView {
    pub(crate) fn set_file_tree_flag(
        &mut self,
        (key, value): (&'static str, Value),
        cx: &mut Context<'_, Self>,
    ) {
        let Some(backend) = self.settings_backend() else {
            tracing::warn!(
                key,
                "the settings are not open; a file tree switch is dropped"
            );
            return;
        };
        if let Err(error) = backend.set(&Key::new(key), value) {
            self.show_notice(NoticeKey::Command, refusal(&error), cx);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_refused_switch_says_the_setting_was_not_saved_and_why() {
        let (kind, text) = refusal(&settings::Error::Unavailable("disk full".into()));
        assert_eq!(kind, ToastKind::Error);
        assert!(text.starts_with("The setting was not saved:") && text.contains("disk full"));
    }
}
