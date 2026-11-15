//! How one tree row takes part in a drag: source, ghost and drop indicators.

use gpui::prelude::FluentBuilder;
use gpui::{App, Div, ParentElement, Stateful, Styled, canvas, div, px};
use ui::{Palette, Theme, ThemeMode, radius, shadow};

use crate::drop_mode::DropMode;
use crate::row_bounds::RowBounds;
use crate::row_id::RowId;

const SOURCE_OPACITY: f32 = 0.4;
const GHOST_SHADOW: f32 = 0.15;
const BORDER: f32 = -1.0;

#[derive(Debug, Clone, Copy, Default, PartialEq)]
pub(crate) struct RowDrag {
    pub(crate) lifted: bool,
    pub(crate) mode: Option<DropMode>,
    pub(crate) ghost: Option<f32>,
}

impl RowDrag {
    pub(crate) fn lifted(width: f32) -> Self {
        Self {
            ghost: Some(width),
            ..Self::default()
        }
    }

    pub(crate) fn theme(
        self,
        cx: &App,
    ) -> Theme {
        let theme = *Theme::of(cx);
        match self.ghost {
            Some(_) => theme.with_mode(ThemeMode::Light),
            None => theme,
        }
    }

    pub(crate) fn style(
        self,
        shell: Stateful<Div>,
        palette: &Palette,
    ) -> Stateful<Div> {
        shell
            .when(self.lifted, |d| d.opacity(SOURCE_OPACITY))
            .when(self.mode == Some(DropMode::Into), |d| {
                d.bg(palette.sidebar_drop_nest())
                    .border_color(palette.sidebar_drop_nest_edge())
            })
            .when_some(self.ghost, |d, width| lift(d, width, palette))
    }
}

fn lift(
    shell: Stateful<Div>,
    width: f32,
    palette: &Palette,
) -> Stateful<Div> {
    let highlight = div()
        .absolute()
        .top_0()
        .left(radius::LG)
        .right(radius::LG)
        .h(px(1.0))
        .bg(palette.elevated_highlight());
    shell
        .m_0()
        .w(px(width))
        .bg(palette.control_surface())
        .border_color(palette.border())
        .shadow(shadow::md(GHOST_SHADOW))
        .text_color(palette.control_foreground())
        .child(highlight)
}

pub(crate) fn tracked(
    shell: Stateful<Div>,
    row: &RowId,
    bounds: &RowBounds,
) -> Stateful<Div> {
    let (row, bounds) = (row.clone(), bounds.clone());
    let probe = canvas(move |at, _, _| bounds.record(&row, at), |_, (), _, _| {})
        .absolute()
        .inset(px(BORDER));
    shell.child(probe)
}

#[cfg(test)]
mod tests {
    use ui::ThemeMode;

    use super::*;

    fn mode_of(
        drag: RowDrag,
        cx: &mut App,
    ) -> ThemeMode {
        drag.theme(cx).mode()
    }

    #[gpui::test]
    fn the_ghost_is_painted_from_the_light_theme_in_every_mode(cx: &mut gpui::TestAppContext) {
        cx.update(|cx| {
            ui::init(cx).expect("ui installs");
            Theme::of(cx).with_mode(ThemeMode::Dark).install(cx);
            assert_eq!(mode_of(RowDrag::lifted(1.0), cx), ThemeMode::Light, "ghost");
            assert_eq!(mode_of(RowDrag::default(), cx), ThemeMode::Dark, "row");
        });
    }
}
