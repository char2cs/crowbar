//! A row counting down to its deletion, with Keep and the drain bar.

use std::time::Instant;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Div, ElementId, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    SharedString, Stateful, StatefulInteractiveElement, Styled, Window, div, px, relative,
};
use ui::font::MONO_FAMILY;
use ui::{Icon, IconName, IconWeight, Palette, Surface, Theme, radius};

use crate::expansion::Expansion;
use crate::hold::Hold;
use crate::intent::Intent;
use crate::removal_tray::RemovalTray;
use crate::row::Row;
use crate::row_action::{RowAction, RowHandler};
use crate::row_glyph::RowGlyph;
use crate::row_kind::RowKind;
use crate::row_report::report;
use crate::row_shell::row_shell;
use crate::titling::Titling;

const BAR_INSET: f32 = 8.0;
const BAR_BOTTOM: f32 = 3.0;
const BAR_HEIGHT: f32 = 2.0;
const FIGURE: f32 = 11.0;

#[derive(IntoElement)]
pub(crate) struct RemovingRow {
    pub(crate) row: Row,
    pub(crate) hold: Hold,
    pub(crate) now: Instant,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for RemovingRow {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let drain = self.hold.drain(self.now, RemovalTray::DRAIN);
        if drain > 0.0 {
            window.request_animation_frame();
        }
        let seconds = self
            .hold
            .seconds_left(self.now)
            .map(|left| figure(left.to_string(), palette));
        let keep = RowAction::Intent(Intent::Keep(self.row.id.clone()));
        let id = ElementId::from(SharedString::from(format!(
            "removing-{}",
            self.row.id.as_str()
        )));
        row_shell(id, self.row.depth, palette)
            .hover(move |style| style.bg(palette.sidebar_element_idle()))
            .child(glyph(&self.row, palette))
            .child(title(&self.row))
            .children(seconds)
            .child(keep_button(palette).on_click(report(&self.handler, keep)))
            .child(drain_bar(drain, palette))
    }
}

fn title(row: &Row) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .text_size(px(13.0))
        .font_weight(FontWeight::MEDIUM)
        .when(row.kind == RowKind::Branch, |d| d.font_family(MONO_FAMILY))
        .when(row.titling == Titling::Provisional, Styled::italic)
        .child(row.label.clone())
}

fn drain_bar(drain: f32, palette: &Palette) -> Div {
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

fn glyph(row: &Row, palette: &Palette) -> impl IntoElement {
    let frame = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(16.0));
    match RowGlyph::of(row, Expansion::Expanded) {
        RowGlyph::Icon(name, weight) => {
            frame.child(Icon::new(name).weight(weight).color(palette.foreground()))
        },
        RowGlyph::Spinner | RowGlyph::Mark => frame.child(Icon::new(IconName::ChatsCircle)),
    }
}

fn figure(text: String, palette: &Palette) -> impl IntoElement {
    div()
        .flex_none()
        .text_size(px(FIGURE))
        .font_family(MONO_FAMILY)
        .text_color(palette.muted_foreground())
        .child(text)
}

fn keep_button(palette: &'static Palette) -> Stateful<Div> {
    let hovered = move |style: gpui::StyleRefinement| {
        style
            .bg(palette.sidebar_element_hover())
            .text_color(palette.foreground())
    };
    let undo = Icon::new(IconName::ArrowCounterClockwise)
        .weight(IconWeight::Bold)
        .size(px(12.0));
    div()
        .id("keep")
        .flex()
        .flex_none()
        .items_center()
        .gap(px(4.0))
        .px(px(6.0))
        .py(px(4.0))
        .rounded(radius::LG)
        .cursor_pointer()
        .text_size(px(12.0))
        .text_color(palette.muted_foreground())
        .hover(hovered)
        .child(undo)
        .child("Keep")
}

#[cfg(test)]
#[path = "removing_row_tests.rs"]
mod tests;
