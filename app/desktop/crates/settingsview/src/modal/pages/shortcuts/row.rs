//! One shortcut: its label, its chord button and a clear control.

use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, SharedString, StatefulInteractiveElement,
    Styled, Window, div, px,
};
use settings::{ChordSource, Shortcut, ShortcutGroup};
use ui::{HoverFill, Icon, IconName, IconWeight, Palette, ScaledText, Surface, Theme, radius};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::spacing;
use crate::intent::Intent;
use chord::Chord;

const HEIGHT: f32 = 49.0;
const BORDER_FADE: f32 = 0.6;
const LABEL_ICON: f32 = 14.0;
const CHORD_HEIGHT: f32 = 32.0;
const CLEAR_BOX: f32 = 20.0;
const CLEAR_ICON: f32 = 16.0;
const CAPTURE_PROMPT: &str = "Press a shortcut...";
const UNBOUND: &str = "Unbound";

pub(crate) fn shortcut_row(
    frame: &Frame<'_>,
    entry: (&ShortcutGroup, &Shortcut),
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let (group, shortcut) = entry;
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let command = shortcut.command.clone();
    let (chord, source) = frame.state.chord(group, shortcut);
    let conflicts = frame.state.conflicts(group, shortcut);
    let note = (!conflicts.is_empty()).then(|| format!("Also used by {}", conflicts.join(", ")));
    let clear = (source == ChordSource::Custom).then(|| clear_button(frame, &command, palette));
    let base = div()
        .flex()
        .items_center()
        .justify_between()
        .h(px(HEIGHT))
        .px(spacing::TIGHT)
        .border_b_1()
        .border_color(palette.border().opacity(BORDER_FADE))
        .font_family(theme.ui_family())
        .text_size(ScaledText::Sm.size(&theme));
    HoverFill::new(
        element_id("shortcut", &command),
        palette.muted().opacity(0.5),
    )
    .apply(base, (window, cx))
    .child(label(shortcut, note, palette))
    .child(
        div()
            .flex()
            .items_center()
            .gap(spacing::SNUG)
            .children(clear)
            .child(chord_button(frame, (&command, chord.as_ref()), &theme)),
    )
}

fn label(
    shortcut: &Shortcut,
    note: Option<String>,
    palette: &Palette,
) -> impl IntoElement {
    div()
        .flex()
        .items_center()
        .gap(spacing::COMFORTABLE)
        .child(
            Icon::new(IconName::Keyboard)
                .weight(IconWeight::Duotone)
                .size(px(LABEL_ICON))
                .color(palette.muted_foreground()),
        )
        .child(
            div()
                .text_color(palette.foreground())
                .child(shortcut.label.clone()),
        )
        .children(note.map(|text| div().text_color(palette.warning()).child(text)))
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

fn chord_button(
    frame: &Frame<'_>,
    (command, chord): (&str, Option<&Chord>),
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let capturing = frame.state.capturing().filter(|target| *target == command);
    let border = capturing.map_or(palette.input(), |_| palette.ring());
    let color = capturing.map_or(palette.muted_foreground(), |_| palette.foreground());
    div()
        .id(element_id("chord", command))
        .flex()
        .items_center()
        .h(px(CHORD_HEIGHT))
        .px(spacing::ROOMY)
        .rounded(radius::LG)
        .border_1()
        .border_color(border)
        .bg(palette.background())
        .font_family(theme.mono_family())
        .text_size(ScaledText::Sm.size(theme))
        .text_color(color)
        .cursor_pointer()
        .on_click(frame.sink.send(Intent::Capture(command.to_owned())))
        .child(chord_text(frame, (command, chord)))
}

fn clear_button(
    frame: &Frame<'_>,
    command: &str,
    palette: &Palette,
) -> impl IntoElement {
    div()
        .id(element_id("clear", command))
        .flex()
        .items_center()
        .justify_center()
        .size(px(CLEAR_BOX))
        .rounded(radius::MD)
        .cursor_pointer()
        .on_click(frame.sink.send(Intent::ClearChord(command.to_owned())))
        .child(
            Icon::new(IconName::ArrowCounterClockwise)
                .size(px(CLEAR_ICON))
                .color(palette.muted_foreground()),
        )
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{KEYBINDING_OVERRIDES, Key, ModifierKey, Pressed, Read as _, Value};

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};
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
}
