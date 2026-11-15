//! One record of a list: icon, text, flags and actions.

use gpui::prelude::FluentBuilder as _;
use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};
use ui::font::{MONO_FAMILY, UI_FAMILY};
use ui::{
    Badge, Button, ButtonSize, ButtonVariant, Icon, IconName, Surface, Switch, Theme, UiText,
};

use crate::element_id::element_id;
use crate::frame::Frame;
use crate::icon_stem::icon_named;
use crate::intent::Intent;
use crate::line::Line;
use crate::record::Record;
use crate::spacing;
use crate::tone_color::tone_color;

const ICON: f32 = 18.0;
const ROW_MIN_HEIGHT: f32 = 44.0;
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
        .gap(spacing::ROOMY)
        .min_h(px(ROW_MIN_HEIGHT))
        .px(spacing::TIGHT)
        .py(spacing::SNUG)
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

fn leading_icon(
    record: &Record,
    palette: &ui::Palette,
) -> Option<Icon> {
    let icon = record.icon.as_deref().and_then(icon_named)?;
    let color = record
        .status
        .map_or(palette.muted_foreground(), |tone| tone_color(tone, palette));
    Some(Icon::new(icon).size(px(ICON)).color(color))
}

fn text_column(
    record: &Record,
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let lines: Vec<AnyElement> = record
        .lines
        .iter()
        .map(|line| line_text(line, theme).into_any_element())
        .collect();
    div()
        .flex()
        .flex_col()
        .gap(spacing::HAIRLINE)
        .flex_1()
        .min_w_0()
        .child(
            div()
                .text_color(palette.foreground())
                .child(record.title.clone()),
        )
        .children(lines)
}

fn line_family(line: &Line) -> &'static str {
    if line.mono {
        return MONO_FAMILY;
    }
    UI_FAMILY
}

fn line_text(
    line: &Line,
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    div()
        .font_family(line_family(line))
        .text_size(UiText::Xs.size(theme))
        .text_color(tone_color(line.tone, palette))
        .child(line.text.clone())
}

fn flag_box(
    frame: &Frame<'_>,
    (source, record): (&str, &Record),
    flag: &crate::record_flag::RecordFlag,
) -> impl IntoElement {
    let id = element_id("flag", &format!("{}-{}", record.id, flag.id));
    let toggled = flag_intent(source, record, &flag.id);
    let sink = frame.sink.clone();
    let switch = Switch::new(id, flag.on)
        .on_change(move |on, window, cx| sink.deliver(toggled(on), (window, cx)));
    div()
        .flex()
        .justify_center()
        .w(px(FLAG_COLUMN))
        .child(switch)
}

fn flag_intent(
    source: &str,
    record: &Record,
    flag: &str,
) -> impl Fn(bool) -> Intent + 'static {
    let (source, record, flag) = (source.to_owned(), record.id.clone(), flag.to_owned());
    move |on| Intent::Flag {
        source: source.clone(),
        record: record.clone(),
        flag: flag.clone(),
        on,
    }
}

fn reorder_intent(
    (source, record): (&str, &Record),
    to: usize,
) -> Intent {
    Intent::Reorder {
        source: source.to_owned(),
        record: record.id.clone(),
        to,
    }
}

fn mover(
    frame: &Frame<'_>,
    (source, record): (&str, &Record),
    (index, count): (usize, usize),
) -> impl IntoElement {
    let step = |name: &str, icon: IconName, to: usize| {
        let reorder = reorder_intent((source, record), to);
        Button::icon_only(element_id(name, &record.id), icon).on_click(frame.sink.send(reorder))
    };
    let up = step("up", IconName::CaretUp, index.saturating_sub(1));
    let down = step("down", IconName::CaretDown, index + 1);
    div()
        .flex()
        .gap(spacing::HAIRLINE)
        .child(up.when(index == 0, Button::disabled))
        .child(down.when(index + 1 >= count, Button::disabled))
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
        .on_click(frame.sink.send(intent))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::catalog::AGENT_RECORDS;
    use crate::intent::Intent;
    use crate::record_command::RecordCommand;
    use crate::test_support::{open, run};

    fn last(harness: &crate::test_support::Harness) -> (String, RecordCommand) {
        let (_, record, command) = harness.backend.commands().pop().unwrap();
        (record, command)
    }

    #[gpui::test]
    fn flags_and_moves_reach_the_backend_as_record_commands(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "providers");
        let flag = Intent::Flag {
            source: AGENT_RECORDS.to_owned(),
            record: "codex".to_owned(),
            flag: "enabled".to_owned(),
            on: true,
        };
        run(&harness, flag, cx);
        assert_eq!(
            last(&harness),
            (
                "codex".to_owned(),
                RecordCommand::SetFlag {
                    flag: "enabled".to_owned(),
                    on: true
                }
            )
        );
        let up = Intent::Reorder {
            source: AGENT_RECORDS.to_owned(),
            record: "codex".to_owned(),
            to: 0,
        };
        run(&harness, up, cx);
        assert_eq!(
            last(&harness),
            ("codex".to_owned(), RecordCommand::Move { to: 0 })
        );
    }
}
