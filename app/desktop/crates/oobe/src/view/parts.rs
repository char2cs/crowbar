use gpui::{Context, Div, ParentElement as _, Pixels, Styled as _, div};
use ui::{Icon, Theme};

use crate::check_status::CheckStatus;
use crate::look::{FAINT, HEADING, STATUS_ICON, color, white};

pub(crate) const DANGER: u32 = 0x00f8_7171;
const WARNING: u32 = 0x00fb_bf24;
const SUCCESS: u32 = 0x0034_d399;

pub(crate) fn column(gap: Pixels) -> Div {
    div()
        .flex()
        .flex_col()
        .items_center()
        .text_center()
        .gap(gap)
}

pub(crate) fn heading<Owner: 'static>(
    text: &'static str,
    cx: &mut Context<'_, Owner>,
) -> Div {
    div()
        .font_family(Theme::of(cx).heading_family())
        .text_size(HEADING)
        .text_color(white(1.0))
        .child(text)
}

pub(crate) fn status_icon(status: CheckStatus) -> Icon {
    let hue = match status {
        CheckStatus::Checking => white(FAINT),
        CheckStatus::Ready => color(SUCCESS),
        CheckStatus::Optional => color(WARNING),
        CheckStatus::Missing => color(DANGER),
    };
    Icon::new(status.icon()).size(STATUS_ICON).color(hue)
}

pub(crate) fn stage(content: Div) -> Div {
    div()
        .absolute()
        .inset_0()
        .flex()
        .items_center()
        .justify_center()
        .child(content)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_stage_is_pinned_to_every_edge_because_a_percent_size_collapses_in_the_live_window() {
        let mut shown = stage(div());
        let inset = &shown.style().inset;
        let pinned = [inset.top, inset.left, inset.right, inset.bottom]
            .iter()
            .all(|edge| *edge == Some(gpui::px(0.0).into()));
        assert!(pinned, "{inset:?}");
    }
}
