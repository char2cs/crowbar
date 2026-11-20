//! The body of the modal: the rail and the page.

use gpui::{
    AnyElement, App, FocusHandle, InteractiveElement, IntoElement, ParentElement, Styled, Window,
    div,
};

use crate::frame::Frame;
use crate::intent::Intent;
use crate::intent::Sink;
use crate::modal::key_routing::Mode;
use crate::modal::key_routing::route;
use crate::modal::pages::content;
use crate::modal::rail::rail;

pub(crate) fn body(
    frame: &Frame<'_>,
    (window, cx): (&mut Window, &mut App),
) -> AnyElement {
    let focus = frame.focus;
    let sink = frame.sink.clone();
    let focused = focus.clone();
    div()
        .id("settings-body")
        .track_focus(focus)
        .flex()
        .size_full()
        .min_w_0()
        .overflow_hidden()
        .on_key_down(move |event, window, cx| {
            let mode = rail_or_page(&focused, window);
            deliver_key(&sink, route(&event.keystroke, mode), (window, cx));
        })
        .child(rail(frame, (&mut *window, &mut *cx)))
        .child(content(frame, frame.scroll, (window, cx)))
        .into_any_element()
}

fn rail_or_page(
    focus: &FocusHandle,
    window: &Window,
) -> Mode {
    if focus.is_focused(window) {
        return Mode::Rail;
    }
    Mode::Page
}

fn deliver_key(
    sink: &Sink,
    intent: Option<Intent>,
    (window, cx): (&mut Window, &mut App),
) {
    let Some(intent) = intent else {
        return;
    };
    cx.stop_propagation();
    sink.deliver(intent, (window, cx));
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{KEYBINDING_OVERRIDES, Key, Read as _, Value};

    use crate::test_support::{Harness, inspect, open};

    fn focus_body(
        harness: &Harness,
        cx: &mut gpui::VisualTestContext,
    ) {
        harness
            .modal
            .update_in(cx, |modal, window, _| modal.focus.focus(window));
        cx.run_until_parked();
    }

    fn page_id(
        harness: &Harness,
        cx: &mut gpui::VisualTestContext,
    ) -> String {
        inspect(harness, cx, |modal| {
            modal
                .state
                .page()
                .map_or_else(String::new, |page| page.id().to_string())
        })
    }

    #[gpui::test]
    fn arrow_keys_walk_the_rail_and_escape_closes(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        focus_body(&harness, cx);
        cx.simulate_keystrokes("down");
        assert_eq!(page_id(&harness, cx), "editor");
        cx.simulate_keystrokes("end");
        assert_eq!(page_id(&harness, cx), "hosts");
        cx.simulate_keystrokes("up");
        assert_eq!(page_id(&harness, cx), "keybindings");
        cx.simulate_keystrokes("escape");
        assert_eq!(harness.closed.get(), 1);
    }

    #[gpui::test]
    fn while_capturing_a_chord_keys_go_to_the_capture(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "keybindings");
        focus_body(&harness, cx);
        let chord = cx.debug_bounds("chord-navigation.toggleConsole").unwrap();
        cx.simulate_click(chord.center(), gpui::Modifiers::none());
        cx.simulate_keystrokes("a");
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .capture_refusal()
            .is_some()));
        cx.simulate_keystrokes("secondary-k");
        let stored = harness.backend.get(&Key::new(KEYBINDING_OVERRIDES));
        let Some(Value::Map(map)) = stored else {
            panic!("no overrides stored: {stored:?}");
        };
        assert_eq!(
            map.get("navigation.toggleConsole"),
            Some(&"mod+k".to_owned())
        );
        assert_eq!(harness.closed.get(), 0);
    }
}
