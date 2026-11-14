//! One record of a list: icon, text, flags and actions.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};
use ui::font::{MONO_FAMILY, UI_FAMILY};
use ui::{
    Badge, Button, ButtonSize, ButtonVariant, Icon, IconName, Surface, Switch, Theme, UiText,
};

use crate::dispatch::send;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::icon_stem::icon_named;
use crate::intent::Intent;
use crate::line::Line;
use crate::record::Record;
use crate::tone_color::tone_color;

const ICON: f32 = 18.0;
pub(crate) const FLAG_COLUMN: f32 = 64.0;

pub(crate) fn record_view(
    frame: &Frame<'_>,
    (source, record, (index, count)): (&str, &Record, (usize, usize)),
    (_, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let leading = leading_icon(record, palette);
    let badge = record
        .badge
        .as_ref()
        .map(|line| Badge::new(line.text.clone()).variant(ui::BadgeVariant::Secondary));
    let text = text_column(record, &theme);
    let flags: Vec<AnyElement> = record
        .flags
        .iter()
        .map(|flag| flag_box(frame, (source, record), flag).into_any_element())
        .collect();
    div()
        .flex()
        .items_center()
        .gap(px(12.0))
        .min_h(px(44.0))
        .px(px(4.0))
        .py(px(6.0))
        .font_family(UI_FAMILY)
        .text_size(UiText::Sm.size(&theme))
        .children(leading)
        .child(text)
        .children(badge)
        .children(flags)
        .children(
            record
                .movable
                .then(|| mover(frame, (source, record), (index, count))),
        )
        .children(
            record
                .action
                .as_ref()
                .map(|action| action_button(frame, (source, record), action)),
        )
}

fn leading_icon(record: &Record, palette: &ui::Palette) -> Option<Icon> {
    let icon = record.icon.as_deref().and_then(icon_named)?;
    let color = record
        .status
        .map_or(palette.muted_foreground(), |tone| tone_color(tone, palette));
    Some(Icon::new(icon).size(px(ICON)).color(color))
}

fn text_column(record: &Record, theme: &Theme) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let lines: Vec<AnyElement> = record
        .lines
        .iter()
        .map(|line| line_text(line, theme).into_any_element())
        .collect();
    div()
        .flex()
        .flex_col()
        .gap(px(2.0))
        .flex_1()
        .min_w_0()
        .child(
            div()
                .text_color(palette.foreground())
                .child(record.title.clone()),
        )
        .children(lines)
}

fn line_text(line: &Line, theme: &Theme) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let family = if line.mono { MONO_FAMILY } else { UI_FAMILY };
    div()
        .font_family(family)
        .text_size(UiText::Xs.size(theme))
        .text_color(tone_color(line.tone, palette))
        .child(line.text.clone())
}

fn flag_box(
    frame: &Frame<'_>,
    (source, record): (&str, &Record),
    flag: &crate::record_flag::RecordFlag,
) -> impl IntoElement {
    let owner = frame.owner.clone();
    let (source, record_id, flag_id) = (source.to_owned(), record.id.clone(), flag.id.clone());
    div().flex().justify_center().w(px(FLAG_COLUMN)).child(
        Switch::new(
            element_id("flag", &format!("{record_id}-{flag_id}")),
            flag.on,
        )
        .on_change(move |on, window, cx| {
            let intent = Intent::Flag {
                source: source.clone(),
                record: record_id.clone(),
                flag: flag_id.clone(),
                on,
            };
            crate::dispatch::deliver(&owner, intent, (window, cx));
        }),
    )
}

fn mover(
    frame: &Frame<'_>,
    (source, record): (&str, &Record),
    (index, count): (usize, usize),
) -> impl IntoElement {
    let step = |name: &str, icon: IconName, to: usize| {
        Button::icon_only(element_id(name, &record.id), icon).on_click(send(
            frame.owner,
            Intent::Reorder {
                source: source.to_owned(),
                record: record.id.clone(),
                to,
            },
        ))
    };
    let up = step("up", IconName::CaretUp, index.saturating_sub(1));
    let down = step("down", IconName::CaretDown, index + 1);
    div()
        .flex()
        .gap(px(2.0))
        .child(if index == 0 { up.disabled() } else { up })
        .child(if index + 1 >= count {
            down.disabled()
        } else {
            down
        })
}

fn action_button(
    frame: &Frame<'_>,
    (source, record): (&str, &Record),
    action: &crate::record_action::RecordAction,
) -> impl IntoElement {
    let id = element_id("record-action", &format!("{}-{}", record.id, action.id));
    let intent = Intent::RunRecord {
        source: source.to_owned(),
        record: record.id.clone(),
        action: action.clone(),
    };
    let icon = icon_named(&action.icon).unwrap_or(IconName::Trash);
    let button = match &action.label {
        Some(label) => Button::new(id, label.clone()).icon(icon),
        None => Button::icon_only(id, icon),
    };
    button
        .variant(ButtonVariant::Outline)
        .size(ButtonSize::Sm)
        .on_click(send(frame.owner, intent))
}

#[cfg(test)]
#[path = "record_view_tests.rs"]
mod tests;
