use gpui::App;

/// Registers what the gpui-kit editor needs before any `EditorView` is drawn: its theme global
/// and the key bindings of its `Input` context.
pub fn init(cx: &mut App) {
    gpui_kit::component::init(cx);
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
