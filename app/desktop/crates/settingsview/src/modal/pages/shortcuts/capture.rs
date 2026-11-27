use gpui::{
    App, Div, FocusHandle, InteractiveElement as _, KeyBinding, NoAction, Stateful, Window,
};

use crate::intent::Intent;
use crate::intent::Sink;
use crate::modal::key_routing::pressed;

const CONTEXT: &str = "SettingsCapture";

pub fn bindings() -> Vec<KeyBinding> {
    ["escape", "tab", "shift-tab"]
        .into_iter()
        .map(|key| KeyBinding::new(key, NoAction, Some(CONTEXT)))
        .collect()
}

pub(crate) fn capturing(
    button: Stateful<Div>,
    focus: &FocusHandle,
    sink: &Sink,
) -> Stateful<Div> {
    let sink = sink.clone();
    button.track_focus(focus).key_context(CONTEXT).on_key_down(
        move |event, window: &mut Window, cx: &mut App| {
            cx.stop_propagation();
            sink.deliver(Intent::Key(pressed(&event.keystroke)), (window, cx));
        },
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_dialog_keys_are_unbound_while_capturing() {
        let keys: Vec<String> = bindings()
            .iter()
            .map(|binding| binding.keystrokes()[0].unparse(gpui::OperatingSystem::Linux))
            .collect();
        assert_eq!(keys, ["escape", "tab", "shift-tab"]);
    }
}
