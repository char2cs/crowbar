use gpui::{App, KeyBinding};

use crate::save_key::{CHORD, CONTEXT, Save};

/// Registers what the gpui-kit editor needs before any `EditorView` is drawn: its theme global,
/// the key bindings of its `Input` context, and ctrl+s to save.
pub fn init(cx: &mut App) {
    gpui_kit::component::init(cx);
    cx.bind_keys([KeyBinding::new(CHORD, Save, Some(CONTEXT))]);
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
