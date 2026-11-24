use gpui::{App, KeyBinding};

use crate::save_key::{CHORD, CONTEXT, Save};
use crate::theme::follow_shell;

/// Registers what the gpui-kit editor needs before any `EditorView` is drawn: its theme global
/// kept following the shell theme, the key bindings of its `Input` context, ctrl+s to save and
/// develop's other editor chords.
pub fn init(cx: &mut App) {
    gpui_kit::component::init(cx);
    follow_shell(cx);
    cx.bind_keys([KeyBinding::new(CHORD, Save, Some(CONTEXT))]);
    cx.bind_keys(crate::keys::bindings());
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn init_installs_the_editor_theme(cx: &mut TestAppContext) {
        cx.update(|cx| {
            init(cx);
            assert!(cx.has_global::<gpui_kit::component::Theme>());
        });
    }
}
