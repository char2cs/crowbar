use gpui::App;

pub(crate) fn keys_bound_to(
    action: &str,
    cx: &App,
) -> String {
    let built = cx.build_action(action, None).unwrap();
    let os = cx.platform_info().os;
    let keymap = cx.key_bindings();
    let keymap = keymap.borrow();
    let binding = keymap.bindings_for_action(built.as_ref()).last().unwrap();
    let strokes: Vec<String> = binding
        .keystrokes()
        .iter()
        .map(|key| key.unparse(os))
        .collect();
    strokes.join(" ")
}
