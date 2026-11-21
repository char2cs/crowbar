//! One record of a list: icon, text, flags and actions.

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, ParentElement, Styled, Window, div, px,
};
use settings::{FlagLook, RecordFlag, Tone};
use ui::{
    Badge, BadgeVariant, Button, ButtonSize, ButtonVariant, Icon, IconName, ScaledText, Surface,
    Switch, Theme,
};

use crate::modal::pages::records::marks::{FLAG_COLUMN, ICON, artwork_image, dot_box, grip};
use crate::modal::pages::records::sortable::sortable;
use settings::{Line, Record};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::icon_named;
use crate::frame::spacing;
use crate::intent::Intent;
use crate::modal::pages::records::listed_record::ListedRecord;
use crate::modal::pages::records::tone_color::tone_color;

const ROW_MIN_HEIGHT: f32 = 44.0;

pub(crate) fn record_view(
    frame: &Frame<'_>,
    entry: ListedRecord<'_>,
    (_, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let ListedRecord { source, record, .. } = entry;
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let leading = leading_icon(record, palette);
    let artwork = record
        .artwork_svg()
        .map(|markup| artwork_image(markup, palette));
    let text = text_column(record, &theme);
    let flags = flag_boxes(frame, entry, palette);
    let grip = record.is_movable().then(|| grip(palette));
    let row = row_shell(source, record)
        .font_family(theme.ui_family())
        .text_size(ScaledText::Sm.size(&theme))
        .children(grip)
        .children(leading)
        .children(artwork)
        .child(text)
        .children(flags)
        .children(
            record
                .offered_action()
                .as_ref()
                .map(|action| action_button(frame, (source, record), action)),
        );
    if record.is_movable() {
        return sortable(row, frame, entry).into_any_element();
    }
    row.into_any_element()
}

fn row_shell(
    source: &str,
    record: &Record,
) -> gpui::Stateful<gpui::Div> {
    let controlled = !record.flags().is_empty() || record.offered_action().is_some();
    let selector = format!("record-{source}-{}", record.id());
    div()
        .id(element_id("record", &format!("{source}-{}", record.id())))
        .debug_selector(move || selector)
        .flex()
        .items_center()
        .gap(spacing::ROOMY)
        .px(spacing::TIGHT)
        .py(spacing::TIGHT)
        .when(controlled, |row| {
            row.min_h(px(ROW_MIN_HEIGHT)).py(spacing::SNUG)
        })
}

fn flag_boxes(
    frame: &Frame<'_>,
    entry: ListedRecord<'_>,
    palette: &ui::Palette,
) -> Vec<AnyElement> {
    entry
        .record
        .flags()
        .iter()
        .map(|flag| flag_box(frame, entry, (flag, palette)))
        .collect()
}

fn leading_icon(
    record: &Record,
    palette: &ui::Palette,
) -> Option<Icon> {
    let icon = record.icon_stem().and_then(icon_named)?;
    let color = record
        .status_tone()
        .map_or(palette.muted_foreground(), |tone| tone_color(tone, palette));
    Some(Icon::new(icon).size(px(ICON)).color(color))
}

fn text_column(
    record: &Record,
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    let lines: Vec<AnyElement> = record
        .lines()
        .iter()
        .map(|line| line_text(line, theme).into_any_element())
        .collect();
    div()
        .flex()
        .flex_col()
        .gap(spacing::HAIRLINE)
        .flex_1()
        .min_w_0()
        .child(title_line(record, theme, palette))
        .children(lines)
}

fn title_line(
    record: &Record,
    theme: &Theme,
    palette: &ui::Palette,
) -> impl IntoElement {
    let aside = record.aside_line().map(|line| {
        div()
            .min_w_0()
            .truncate()
            .text_size(ScaledText::Xs.size(theme))
            .text_color(palette.muted_foreground())
            .child(line.text().to_owned())
    });
    let badge = record
        .badge_line()
        .map(|line| Badge::new(line.text().to_owned()).variant(badge_variant(line.tone())));
    div()
        .flex()
        .items_center()
        .gap(spacing::COMFORTABLE)
        .min_w_0()
        .child(
            div()
                .flex_none()
                .text_color(palette.foreground())
                .child(record.title().to_owned()),
        )
        .children(aside)
        .children(badge)
}

fn badge_variant(tone: Tone) -> BadgeVariant {
    match tone {
        Tone::Success => BadgeVariant::Success,
        Tone::Warning => BadgeVariant::Warning,
        Tone::Error => BadgeVariant::Error,
        Tone::Neutral | Tone::Pending => BadgeVariant::Secondary,
    }
}

fn line_family(
    line: &Line,
    theme: &Theme,
) -> &'static str {
    if line.is_mono() {
        return theme.mono_family();
    }
    theme.ui_family()
}

fn line_text(
    line: &Line,
    theme: &Theme,
) -> impl IntoElement {
    let palette = theme.palette(Surface::Window);
    div()
        .font_family(line_family(line, theme))
        .text_size(ScaledText::Xs.size(theme))
        .text_color(tone_color(line.tone(), palette))
        .child(line.text().to_owned())
}

fn flag_box(
    frame: &Frame<'_>,
    entry: ListedRecord<'_>,
    (flag, palette): (&RecordFlag, &ui::Palette),
) -> AnyElement {
    let ListedRecord { source, record, .. } = entry;
    let id = element_id("flag", &format!("{}-{}", record.id(), flag.id()));
    if flag.look() == FlagLook::Dot {
        return dot_box(flag, palette).into_any_element();
    }
    let toggled = flag_intent(source, record, flag.id());
    let sink = frame.sink.clone();
    let switch = Switch::new(id, flag.is_on())
        .on_change(move |on, window, cx| sink.deliver(toggled(bool::from(on)), (window, cx)));
    div()
        .flex()
        .justify_center()
        .w(px(FLAG_COLUMN))
        .child(switch)
        .into_any_element()
}

fn flag_intent(
    source: &str,
    record: &Record,
    flag: &str,
) -> impl Fn(bool) -> Intent + 'static {
    let (source, record, flag) = (source.to_owned(), record.id().to_owned(), flag.to_owned());
    move |on| Intent::Flag {
        source: source.clone(),
        record: record.clone(),
        flag: flag.clone(),
        on,
    }
}

fn action_button(
    frame: &Frame<'_>,
    (source, record): (&str, &Record),
    action: &settings::RecordAction,
) -> impl IntoElement {
    let id = element_id("record-action", &format!("{}-{}", record.id(), action.id()));
    let intent = Intent::RunRecord {
        source: source.to_owned(),
        record: record.id().to_owned(),
        action: action.clone(),
    };
    let icon = icon_named(action.icon()).unwrap_or(IconName::Trash);
    let button = match action.label() {
        Some(label) => Button::new(id, label.to_owned()).icon(icon),
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
    use settings::{AGENT_RECORDS, RecordCommand};

    use crate::intent::Intent;
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
