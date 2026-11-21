//! One shortcut: label with its tags, the chord button and a Reset for a custom chord.
//! Spec: web/src/features/settings/components/tabs/keybindings-settings.tsx.

use gpui::{
    AnyElement, App, Div, FocusHandle, InteractiveElement, IntoElement, ParentElement,
    SharedString, Stateful, StatefulInteractiveElement, Styled, Window, div, px,
};
use settings::{ChordSource, Shortcut, ShortcutGroup};
use ui::{
    Button, ButtonSize, ButtonVariant, Icon, IconName, Palette, ScaledText, Surface, Theme, radius,
};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::spacing;
use crate::intent::Intent;
use crate::modal::pages::shortcuts::capture::capturing;
use crate::modal::pages::shortcuts::chord_face::ChordFace;
use crate::modal::pages::shortcuts::chord_look::ChordLook;
use chord::Chord;

const LABEL_ICON: f32 = 14.0;
const WARNING_ICON: f32 = 14.0;
const CHORD_HEIGHT: f32 = 32.0;
const OUTLINE_FILL: f32 = 0.32;
const CHORD_MIN_WIDTH: f32 = 88.0;
const DISABLED_OPACITY: f32 = 0.64;
const CHIP_X: f32 = 6.0;
const CHIP_Y: f32 = 2.0;
const CAPTURE_PROMPT: &str = "Press keys\u{2026}";
const UNBOUND: &str = "Unbound";
const DISPLAY_ONLY: &str = "display-only";
const CUSTOM: &str = "custom";
const PRESET: &str = "preset";
const RESET: &str = "Reset";

pub(crate) fn shortcut_row(
    frame: &Frame<'_>,
    entry: (&ShortcutGroup, &Shortcut),
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let (group, shortcut) = entry;
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let command = shortcut.command().to_owned();
    let (chord, source) = frame.state.chord(group, shortcut);
    let conflicts = frame.state.conflicts(group, shortcut);
    let warning = (!conflicts.is_empty()).then(|| warning(&conflicts, palette));
    let reset = (source == ChordSource::Custom).then(|| reset_button(frame, &command));
    let face = ChordFace {
        text: chord_text(frame, (&command, chord.as_ref())),
        look: look_of(frame, &command, &conflicts),
        focus: chord_focus(&command, (window, cx)),
    };
    let chord_box = chord_button(frame, shortcut, face, &theme);
    let selector = format!("shortcut-{command}");
    div()
        .debug_selector(move || selector)
        .flex()
        .items_center()
        .justify_between()
        .gap(spacing::ROOMY)
        .px(spacing::TIGHT)
        .py(spacing::COMFORTABLE)
        .font_family(theme.ui_family())
        .text_size(ScaledText::Sm.size(&theme))
        .child(label(shortcut, source, &theme))
        .child(
            div()
                .flex()
                .items_center()
                .gap(spacing::COMFORTABLE)
                .children(warning)
                .child(chord_box)
                .children(reset),
        )
}

fn label(
    shortcut: &Shortcut,
    source: ChordSource,
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let small = ScaledText::Xs.size(theme);
    let chip = shortcut.is_display_only().then(|| {
        div()
            .px(px(CHIP_X))
            .py(px(CHIP_Y))
            .rounded(radius::SM)
            .bg(palette.muted())
            .text_size(small)
            .text_color(palette.muted_foreground())
            .child(DISPLAY_ONLY)
    });
    let tag = source_tag(source).map(|text| {
        div()
            .text_size(small)
            .text_color(palette.accent_foreground())
            .child(text)
    });
    div()
        .flex()
        .items_center()
        .gap(spacing::COMFORTABLE)
        .min_w_0()
        .child(
            Icon::new(IconName::Keyboard)
                .size(px(LABEL_ICON))
                .color(palette.muted_foreground()),
        )
        .child(
            div()
                .text_color(palette.foreground())
                .child(shortcut.label().to_owned()),
        )
        .children(chip)
        .children(tag)
}

fn source_tag(source: ChordSource) -> Option<&'static str> {
    match source {
        ChordSource::Custom => Some(CUSTOM),
        ChordSource::Preset => Some(PRESET),
        _ => None,
    }
}

fn warning(
    conflicts: &[String],
    palette: &Palette,
) -> impl IntoElement {
    let text: SharedString = format!("Also bound to: {}", conflicts.join(", ")).into();
    ui::Tooltip::new(
        SharedString::from(format!("conflict-{text}")),
        text,
        Icon::new(IconName::Warning)
            .size(px(WARNING_ICON))
            .color(palette.destructive()),
    )
}

fn chord_text(
    frame: &Frame<'_>,
    (command, chord): (&str, Option<&Chord>),
) -> SharedString {
    if frame.state.capturing() != Some(command) {
        return chord
            .map_or(UNBOUND.to_owned(), |chord| frame.state.label(chord))
            .into();
    }
    frame
        .state
        .capture_refusal()
        .map_or(CAPTURE_PROMPT.to_owned(), |refusal| {
            refusal.message().to_owned()
        })
        .into()
}

fn look_of(
    frame: &Frame<'_>,
    command: &str,
    conflicts: &[String],
) -> ChordLook {
    if frame.state.capturing() == Some(command) {
        return ChordLook::Capturing;
    }
    if !conflicts.is_empty() {
        return ChordLook::Conflict;
    }
    ChordLook::Idle
}

fn chord_colors(
    palette: &Palette,
    look: ChordLook,
) -> (gpui::Hsla, gpui::Hsla, gpui::Hsla) {
    match look {
        ChordLook::Capturing => (
            palette.primary(),
            palette.primary(),
            palette.primary_foreground(),
        ),
        ChordLook::Conflict => (
            palette.destructive(),
            palette.input().opacity(OUTLINE_FILL),
            palette.destructive(),
        ),
        ChordLook::Idle => (
            palette.input(),
            palette.input().opacity(OUTLINE_FILL),
            palette.foreground(),
        ),
    }
}

fn chord_focus(
    command: &str,
    (window, cx): (&mut Window, &mut App),
) -> FocusHandle {
    window
        .use_keyed_state(element_id("chord-focus", command), cx, |_, cx| {
            cx.focus_handle()
        })
        .read(cx)
        .clone()
}

fn chord_button(
    frame: &Frame<'_>,
    shortcut: &Shortcut,
    face: ChordFace,
    theme: &Theme,
) -> AnyElement {
    let command = shortcut.command();
    let look = face.look;
    let focus = face.focus.clone();
    let base = chord_box(command, face, theme);
    if shortcut.is_display_only() {
        return base.opacity(DISABLED_OPACITY).into_any_element();
    }
    let sink = frame.sink.clone();
    let intent = Intent::Capture(command.to_owned());
    let held = focus.clone();
    let base = base
        .track_focus(&focus)
        .cursor_pointer()
        .on_click(move |_, window, cx| {
            window.focus(&held, cx);
            sink.deliver(intent.clone(), (window, cx));
        });
    if look != ChordLook::Capturing {
        return base.into_any_element();
    }
    capturing(base, &focus, frame.sink).into_any_element()
}

fn chord_box(
    command: &str,
    face: ChordFace,
    theme: &Theme,
) -> Stateful<Div> {
    let palette = theme.palette(Surface::Window);
    let (border, fill, ink) = chord_colors(palette, face.look);
    div()
        .id(element_id("chord", command))
        .debug_selector({
            let selector = format!("chord-{command}");
            move || selector
        })
        .flex()
        .items_center()
        .justify_center()
        .h(px(CHORD_HEIGHT))
        .min_w(px(CHORD_MIN_WIDTH))
        .px(spacing::COMFORTABLE)
        .rounded(radius::LG)
        .border_1()
        .border_color(border)
        .bg(fill)
        .font_family(theme.mono_family())
        .text_size(ScaledText::Sm.size(theme))
        .text_color(ink)
        .child(face.text)
}

fn reset_button(
    frame: &Frame<'_>,
    command: &str,
) -> impl IntoElement {
    Button::new(element_id("reset-chord", command), RESET)
        .variant(ButtonVariant::Ghost)
        .size(ButtonSize::Xs)
        .on_click(frame.sink.send(Intent::ClearChord(command.to_owned())))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{KEYBINDING_OVERRIDES, Key, ModifierKey, Pressed, Read as _, Value};

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, open_floating, run};
    use chord::Primary;

    const COMMAND: &str = "navigation.toggleConsole";

    fn overrides(harness: &crate::test_support::Harness) -> Option<Value> {
        harness.backend.get(&Key::new(KEYBINDING_OVERRIDES))
    }

    #[gpui::test]
    fn a_modified_key_is_stored_and_clearing_it_restores_the_default(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "keybindings");
        run(&harness, Intent::Capture(COMMAND.to_owned()), cx);
        assert_eq!(
            inspect(&harness, cx, |modal| modal
                .state
                .capturing()
                .map(str::to_owned)),
            Some(COMMAND.to_owned())
        );
        run(
            &harness,
            Intent::Key(Pressed::new(
                "k",
                &[ModifierKey::primary(Primary::native())],
            )),
            cx,
        );
        assert!(matches!(overrides(&harness), Some(Value::Map(map)) if map.len() == 1));
        run(&harness, Intent::ClearChord(COMMAND.to_owned()), cx);
        assert!(matches!(overrides(&harness), Some(Value::Map(map)) if map.is_empty()));
    }

    #[gpui::test]
    fn a_bare_printable_key_is_refused_and_stores_nothing(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "keybindings");
        run(&harness, Intent::Capture(COMMAND.to_owned()), cx);
        run(&harness, Intent::Key(Pressed::new("a", &[])), cx);
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .capture_refusal()
            .is_some()));
        assert_eq!(overrides(&harness), None);
        run(&harness, Intent::Key(Pressed::new("escape", &[])), cx);
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .capturing()
            .is_none()));
    }

    #[gpui::test]
    fn a_chord_clicked_in_the_open_dialog_takes_the_next_typed_chord(cx: &mut TestAppContext) {
        let (harness, mut owner, mut top) = open_floating(cx, "keybindings");
        harness
            .modal
            .update_in(&mut owner, |modal, window, cx| modal.focus(window, cx));
        owner.run_until_parked();
        let chord = top.debug_bounds("chord-navigation.toggleConsole").unwrap();
        top.simulate_click(chord.center(), gpui::Modifiers::none());
        top.simulate_keystrokes("escape");
        assert_eq!(harness.closed.get(), 0, "escape cancels the capture only");
        top.simulate_click(chord.center(), gpui::Modifiers::none());
        top.simulate_keystrokes("ctrl-shift-k");
        assert!(matches!(overrides(&harness), Some(Value::Map(map)) if map.len() == 1));
    }
}
