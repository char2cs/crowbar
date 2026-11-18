//! One record of a list: icon, text, flags and actions.

use gpui::AppContext as _;
use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement,
    Styled, Window, div, px,
};
use settings::{FlagLook, RecordFlag};
use ui::{
    Badge, Button, ButtonSize, ButtonVariant, DragGhost, Icon, IconName, Surface, Switch, Theme,
    UiText,
};

use crate::modal::pages::records::drag_record::DragRecord;
use crate::modal::pages::records::marks::{FLAG_COLUMN, ICON, artwork_image, dot_box, grip};
use settings::{Line, Record};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::icon_named;
use crate::frame::spacing;
use crate::intent::Intent;
use crate::modal::pages::records::record_ref::RecordRef;
use crate::modal::pages::records::tone_color::tone_color;

const DROP_LINE: f32 = 2.0;
const GHOST_WIDTH: f32 = 220.0;
const ROW_MIN_HEIGHT: f32 = 44.0;

pub(crate) fn record_view(
    frame: &Frame<'_>,
    entry: RecordRef<'_>,
    (_, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let RecordRef { source, record, .. } = entry;
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let leading = leading_icon(record, palette);
    let artwork = record.artwork.as_deref().map(artwork_image);
    let badge = record
        .badge
        .as_ref()
        .map(|line| Badge::new(line.text.clone()).variant(ui::BadgeVariant::Secondary));
    let text = text_column(record, &theme);
    let flags = flag_boxes(frame, entry, palette);
    let grip = record.movable.then(|| grip(palette));
    let row = div()
        .id(element_id("record", &format!("{source}-{}", record.id)))
        .flex()
        .items_center()
        .gap(spacing::ROOMY)
        .min_h(px(ROW_MIN_HEIGHT))
        .px(spacing::TIGHT)
        .py(spacing::SNUG)
        .font_family(theme.ui_family())
        .text_size(UiText::Sm.size(&theme))
        .children(grip)
        .children(leading)
        .children(artwork)
        .child(text)
        .children(badge)
        .children(flags)
        .children(
            record
                .action
                .as_ref()
                .map(|action| action_button(frame, (source, record), action)),
        );
    if record.movable {
        return draggable(row, frame, entry).into_any_element();
    }
    row.into_any_element()
}

fn flag_boxes(
    frame: &Frame<'_>,
    entry: RecordRef<'_>,
    palette: &ui::Palette,
) -> Vec<AnyElement> {
    entry
        .record
        .flags
        .iter()
        .map(|flag| flag_box(frame, entry, (flag, palette)))
        .collect()
}

fn draggable(
    row: gpui::Stateful<gpui::Div>,
    frame: &Frame<'_>,
    entry: RecordRef<'_>,
) -> impl IntoElement {
    let RecordRef {
        source,
        record,
        index,
    } = entry;
    let drag = DragRecord {
        source: source.to_owned(),
        record: record.id.clone(),
        title: record.title.clone(),
    };
    let sink = frame.sink.clone();
    let target = source.to_owned();
    row.cursor_grab()
        .on_drag(drag, |drag, _, _, cx| {
            cx.new(|_| DragGhost::new(drag.title.clone(), GHOST_WIDTH))
        })
        .drag_over::<DragRecord>(|row, _, _, cx| {
            let line = Theme::of(cx).palette(Surface::Window).ring();
            row.border_t(px(DROP_LINE)).border_color(line)
        })
        .on_drop::<DragRecord>(move |drag, window, cx| {
            drag.deliver(&sink, (&target, index), (window, cx));
        })
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

fn line_family(
    line: &Line,
    theme: &Theme,
) -> &'static str {
    if line.mono {
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
        .text_size(UiText::Xs.size(theme))
        .text_color(tone_color(line.tone, palette))
        .child(line.text.clone())
}

fn flag_box(
    frame: &Frame<'_>,
    entry: RecordRef<'_>,
    (flag, palette): (&RecordFlag, &ui::Palette),
) -> AnyElement {
    let RecordRef { source, record, .. } = entry;
    let id = element_id("flag", &format!("{}-{}", record.id, flag.id));
    if flag.look == FlagLook::Dot {
        return dot_box(flag, palette).into_any_element();
    }
    let toggled = flag_intent(source, record, &flag.id);
    let sink = frame.sink.clone();
    let switch = Switch::new(id, flag.on)
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
    let (source, record, flag) = (source.to_owned(), record.id.clone(), flag.to_owned());
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
