//! The 44px space switcher: active space name, add button and the other spaces.

use gpui::{
    AnyElement, Context, Div, ElementId, FontWeight, InteractiveElement, IntoElement,
    ParentElement, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::font::HEADING_FAMILY;
use ui::{Palette, Surface, Theme, Tooltip, hover_fill, radius};

use crate::command::Command;
use crate::hover_glide;
use crate::icon_button::IconButton;
use crate::intent::Intent;
use crate::mark_view::mark_view;
use crate::metrics::SWITCHER_HEIGHT;
use crate::project::Project;
use crate::row_action::RowAction;
use crate::sidebar::Sidebar;
use crate::tile_shape::TileShape;

const TILE: f32 = 28.0;
const MARK: f32 = 20.0;
const NAME_SIZE: f32 = 19.0;
const NAME_LINE: f32 = 24.0;
const OTHER_MARK_ALPHA: f32 = 0.8;

impl Sidebar {
    pub(crate) fn switcher(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let projects = self.data.projects();
        let active = self.data.active_project()?;
        let current = projects.iter().find(|project| project.id == active)?;
        let glide = hover_glide::track("switcher", window, cx);
        let reveal = match projects.len() {
            0 | 1 => 1.0,
            _ => hover_glide::value(&glide, window, cx),
        };
        let others: Vec<AnyElement> = projects
            .iter()
            .filter(|project| project.id != current.id)
            .map(|project| Self::other_mark(project, palette, window, cx))
            .collect();
        let block = active_block(current, Self::add_button(cx), reveal, palette);
        Some(
            div()
                .id("switcher")
                .flex()
                .flex_none()
                .items_center()
                .gap(px(8.0))
                .h(px(SWITCHER_HEIGHT))
                .px(px(12.0))
                .overflow_hidden()
                .on_hover(hover_glide::listener(&glide))
                .child(block)
                .children(others)
                .into_any_element(),
        )
    }

    fn add_button(cx: &mut Context<'_, Self>) -> IconButton {
        let action = RowAction::Intent(Intent::StartCreateSpace);
        IconButton::new("sidebar-add-space", ui::IconName::Plus)
            .tooltip("Add space")
            .shape(TileShape::Tile)
            .on_click(cx.listener(move |this, _, window, cx| {
                this.act(action.clone(), window, cx);
            }))
    }

    fn other_mark(
        project: &Project,
        palette: &'static Palette,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let group = SharedString::from(format!("mark-{}", project.id.as_str()));
        let fill = palette.sidebar_element_hover();
        let tile = hover_fill(
            mark_tile(project, &group, palette),
            ElementId::from(group.clone()),
            fill,
            (window, &mut **cx),
        );
        let action = RowAction::Intent(Intent::Send(Command::SelectProject(project.id.clone())));
        let button = tile.on_click(cx.listener(move |this, _, window, cx| {
            this.act(action.clone(), window, cx);
        }));
        let id = ElementId::from(SharedString::from(format!("tip-{group}")));
        Tooltip::new(id, project.name.clone(), button).into_any_element()
    }
}

fn mark_tile(project: &Project, group: &SharedString, palette: &Palette) -> Div {
    let inner = div()
        .opacity(OTHER_MARK_ALPHA)
        .group_hover(group.clone(), |style| style.opacity(1.0))
        .child(mark_view(&project.mark, px(MARK), palette));
    div()
        .group(group.clone())
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(TILE))
        .rounded(radius::MD)
        .cursor_pointer()
        .child(inner)
}

fn active_block(current: &Project, add: IconButton, reveal: f32, palette: &Palette) -> Div {
    let tile = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(TILE))
        .ml(px(-4.0))
        .rounded(radius::MD)
        .child(mark_view(&current.mark, px(MARK), palette));
    let name = div()
        .flex_1()
        .min_w_0()
        .truncate()
        .font_family(HEADING_FAMILY)
        .text_size(px(NAME_SIZE))
        .line_height(px(NAME_LINE))
        .font_weight(FontWeight::NORMAL)
        .text_color(palette.foreground())
        .child(current.name.clone());
    let slot = div()
        .flex_none()
        .overflow_hidden()
        .w(px(TILE * reveal))
        .ml(px(-4.0 * (1.0 - reveal)))
        .opacity(reveal)
        .child(add);
    div()
        .flex()
        .flex_1()
        .min_w_0()
        .items_center()
        .gap(px(4.0))
        .child(tile)
        .child(name)
        .child(slot)
}

#[cfg(test)]
#[path = "switcher_tests.rs"]
mod tests;
