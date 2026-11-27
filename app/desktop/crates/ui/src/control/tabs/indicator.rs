use gpui::{Div, InteractiveElement as _, ParentElement, Styled, div, px, relative};

use crate::control::tabs::tabs_style::TabsStyle;
use crate::foundation::Palette;

pub(crate) const GAP: f32 = 2.0;

pub(crate) fn tab_indicator(
    style: TabsStyle,
    palette: &Palette,
    count: u16,
    position: f32,
) -> Div {
    let slots = f32::from(count.max(1));
    let pill = style.pill(palette).h_full().mr(px(GAP));
    let slot = div()
        .absolute()
        .top_0()
        .h_full()
        .w(relative(1.0 / slots))
        .left(relative(position / slots))
        .debug_selector(|| "tab-indicator-slot".into())
        .child(pill);
    div()
        .absolute()
        .top_0()
        .bottom_0()
        .left_0()
        .right(px(-GAP))
        .child(slot)
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement, Styled, TestAppContext, div, px};

    use super::*;
    use crate::foundation::Surface;
    use crate::foundation::Theme;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn pill_is_one_slot_wide_and_moves_with_the_position(cx: &mut TestAppContext) {
        let palette = Theme::default().palette(Surface::Window);
        let cx = open(cx, move |_, _| {
            let pill = tab_indicator(TabsStyle::Pill, palette, 4, 2.0);
            div().relative().w(px(400.0)).h(px(32.0)).child(pill)
        });
        let slot = cx.debug_bounds("tab-indicator-slot").unwrap_or_default();
        let frame = cx.debug_bounds("probe").unwrap_or_default();
        assert!((f32::from(slot.size.width) - 100.5).abs() < 0.5, "{slot:?}");
        assert!(
            (f32::from(slot.origin.x - frame.origin.x) - 201.0).abs() < 0.5,
            "{slot:?} in {frame:?}"
        );
    }
}
