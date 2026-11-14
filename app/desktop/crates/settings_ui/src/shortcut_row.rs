//! One shortcut: its label, its chord button and a clear control.

use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, SharedString, StatefulInteractiveElement,
    Styled, Window, div, px,
};
use ui::font::{MONO_FAMILY, UI_FAMILY};
use ui::{Icon, IconName, IconWeight, Palette, Surface, Theme, UiText, hover_fill, radius};

use crate::chord::Chord;
use crate::chord_source::ChordSource;
use crate::dispatch::send;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;

const HEIGHT: f32 = 49.0;

pub(crate) fn shortcut_row(
    frame: &Frame<'_>,
    (group, shortcut): (&ShortcutGroup, &Shortcut),
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
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
        .px(px(4.0))
        .border_b_1()
        .border_color(palette.border().opacity(0.6))
        .font_family(UI_FAMILY)
        .text_size(UiText::Sm.size(&theme));
    hover_fill(
        base,
        element_id("shortcut", &command),
        palette.muted().opacity(0.5),
        (window, cx),
    )
    .child(label(shortcut, note, palette))
    .child(
        div()
            .flex()
            .items_center()
            .gap(px(6.0))
            .children(clear)
            .child(chord_button(frame, (&command, chord.as_ref()), &theme)),
    )
}

fn label(shortcut: &Shortcut, note: Option<String>, palette: &Palette) -> impl IntoElement {
    div()
        .flex()
        .items_center()
        .gap(px(8.0))
        .child(
            Icon::new(IconName::Keyboard)
                .weight(IconWeight::Duotone)
                .size(px(14.0))
                .color(palette.muted_foreground()),
        )
        .child(
            div()
                .text_color(palette.foreground())
                .child(shortcut.label.clone()),
        )
        .children(note.map(|text| div().text_color(palette.warning()).child(text)))
}

fn chord_button(
    frame: &Frame<'_>,
    (command, chord): (&str, Option<&Chord>),
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let capturing = frame.state.capturing() == Some(command);
    let text: SharedString = if capturing {
        frame
            .state
            .capture_refusal()
            .map_or("Press a shortcut...".to_owned(), |refusal| {
                refusal.message().to_owned()
            })
    } else {
        chord.map_or("Unbound".to_owned(), |chord| frame.state.label(chord))
    }
    .into();
    let border = if capturing {
        palette.ring()
    } else {
        palette.input()
    };
    let color = if capturing {
        palette.foreground()
    } else {
        palette.muted_foreground()
    };
    div()
        .id(element_id("chord", command))
        .flex()
        .items_center()
        .h(px(32.0))
        .px(px(12.0))
        .rounded(radius::LG)
        .border_1()
        .border_color(border)
        .bg(palette.background())
        .font_family(MONO_FAMILY)
        .text_size(UiText::Sm.size(theme))
        .text_color(color)
        .cursor_pointer()
        .on_click(send(frame.owner, Intent::Capture(command.to_owned())))
        .child(text)
}

fn clear_button(frame: &Frame<'_>, command: &str, palette: &Palette) -> impl IntoElement {
    div()
        .id(element_id("clear", command))
        .flex()
        .items_center()
        .justify_center()
        .size(px(20.0))
        .rounded(radius::MD)
        .cursor_pointer()
        .on_click(send(frame.owner, Intent::ClearChord(command.to_owned())))
        .child(
            Icon::new(IconName::ArrowCounterClockwise)
                .size(px(16.0))
                .color(palette.muted_foreground()),
        )
}

#[cfg(test)]
#[path = "shortcut_row_tests.rs"]
mod tests;
