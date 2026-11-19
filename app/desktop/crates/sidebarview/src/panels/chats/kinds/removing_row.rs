//! A row counting down to its deletion, with the keep button and the drain bar.

use std::sync::Arc;
use std::time::Instant;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Div, ElementId, Font, FontFeatures, FontStyle, FontWeight, InteractiveElement,
    IntoElement, ParentElement, RenderOnce, SharedString, Stateful, StatefulInteractiveElement,
    Styled, Window, div, px, relative,
};
use sidebar::{Expansion, Hold, Intent, RemovalTray, Row, RowKind, Titling};
use ui::{Icon, IconName, IconWeight, Palette, Surface, Theme, radius, spacing};

use crate::panels::chats::parts::RowGlyph;
use crate::panels::chats::parts::report;
use crate::panels::chats::parts::row_shell;
use crate::sidebar::{ICON_LARGE, ICON_SMALL, TEXT_BODY};
use crate::sidebar::{RowAction, RowHandler};

const BAR_INSET: f32 = 8.0;
const BAR_BOTTOM: f32 = 3.0;
const BAR_HEIGHT: f32 = 2.0;
const FIGURE: f32 = 11.0;
const TABULAR: &str = "tnum";

#[derive(IntoElement)]
pub(crate) struct RemovingRow {
    pub(crate) row: Row,
    pub(crate) hold: Hold,
    pub(crate) now: Instant,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for RemovingRow {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = Theme::of(cx);
        let (mono, palette) = (theme.mono_family(), theme.palette(Surface::Chrome));
        let drain = self.hold.drain(self.now, RemovalTray::DRAIN);
        if drain > 0.0 {
            window.request_animation_frame();
        }
        let seconds = figure(
            self.hold.seconds_left(self.now).to_string(),
            theme.ui_family(),
            palette,
        );
        let keep = RowAction::Intent(Intent::Keep(self.row.id.clone()));
        let id = ElementId::from(SharedString::from(format!(
            "removing-{}",
            self.row.id.as_str()
        )));
        row_shell(id, self.row.depth, palette)
            .hover(move |style| style.bg(palette.sidebar_element_idle()))
            .child(glyph(&self.row, palette))
            .child(title(&self.row, mono))
            .child(seconds)
            .child(keep_button(palette).on_click(report(&self.handler, keep)))
            .child(drain_bar(drain, palette))
    }
}

fn title(
    row: &Row,
    mono: &'static str,
) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .text_size(TEXT_BODY)
        .font_weight(FontWeight::MEDIUM)
        .when(row.kind == RowKind::Branch, |d| d.font_family(mono))
        .when(row.titling == Titling::Provisional, Styled::italic)
        .child(row.label.clone())
}

fn drain_bar(
    drain: f32,
    palette: &Palette,
) -> Div {
    let fill = div()
        .h_full()
        .w(relative(drain))
        .rounded(radius::FULL)
        .bg(palette.destructive());
    div()
        .absolute()
        .left(px(BAR_INSET))
        .right(px(BAR_INSET))
        .bottom(px(BAR_BOTTOM))
        .h(px(BAR_HEIGHT))
        .child(fill)
}

fn glyph(
    row: &Row,
    palette: &Palette,
) -> impl IntoElement {
    let frame = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(ICON_LARGE);
    match RowGlyph::of(row, Expansion::Expanded) {
        RowGlyph::Icon(name, weight) => {
            frame.child(Icon::new(name).weight(weight).color(palette.foreground()))
        },
        RowGlyph::Spinner | RowGlyph::Mark => frame.child(Icon::new(IconName::ChatsCircle)),
    }
}

fn figure(
    text: String,
    family: &'static str,
    palette: &Palette,
) -> Div {
    let tabular = Font {
        family: family.into(),
        features: FontFeatures(Arc::new(vec![(TABULAR.into(), 1)])),
        fallbacks: None,
        weight: FontWeight::NORMAL,
        style: FontStyle::Normal,
    };
    div()
        .flex_none()
        .text_size(px(FIGURE))
        .font(tabular)
        .text_color(palette.muted_foreground())
        .child(text)
}

fn keep_button(palette: &'static Palette) -> Stateful<Div> {
    let hovered = move |style: gpui::StyleRefinement| {
        style
            .bg(palette.sidebar_element_hover())
            .text_color(palette.foreground())
    };
    let undo = Icon::new(IconName::ArrowUUpLeft)
        .weight(IconWeight::Bold)
        .size(ICON_SMALL);
    div()
        .id("keep")
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .p(spacing::STEP_1_5)
        .rounded(radius::LG)
        .cursor_pointer()
        .text_color(palette.muted_foreground())
        .hover(hovered)
        .child(undo)
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use gpui::TestAppContext;

    use std::rc::Rc;

    use super::*;
    use sidebar::{Deadline, RemovalKind, RowId};

    use crate::test_support::{chat, measure, mount, recording_handler};

    #[gpui::test]
    fn a_held_row_keeps_its_height_while_it_drains(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        let now = Instant::now();
        let hold = Hold {
            row: RowId::from("chat"),
            kind: RemovalKind::Chat,
            label: "x".into(),
            deadline: Deadline::At(now + Duration::from_secs(4)),
        };
        let bounds = measure(cx, move |_, _| RemovingRow {
            row: chat("x"),
            hold: hold.clone(),
            now,
            handler: Rc::clone(&handler),
        });
        assert_eq!(f32::from(bounds.size.height), 38.0);
    }

    #[gpui::test]
    fn keep_is_a_square_icon_button_with_no_label(cx: &mut TestAppContext) {
        let palette = Theme::default().palette(Surface::Chrome);
        let cx = mount(cx, move |_, _| {
            div()
                .flex()
                .child(keep_button(palette).debug_selector(|| "keep".into()))
        });
        let bounds = cx.debug_bounds("keep").unwrap_or_default();
        assert_eq!(
            f32::from(bounds.size.width),
            24.0,
            "12px glyph, 6px padding"
        );
    }
}
