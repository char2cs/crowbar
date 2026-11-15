//! The 44px space switcher: active space name, add button and the other spaces.

use gpui::{
    AnyElement, Context, Div, ElementId, Entity, FontWeight, InteractiveElement, IntoElement,
    ParentElement, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Palette, Placement, Popover, Surface, Theme, Tooltip, hover_fill, radius};

use crate::command::Command;
use crate::edit_pencil::{GROUP, edit_pencil};
use crate::hover_glide;
use crate::icon_button::IconButton;
use crate::icon_editor::IconEditor;
use crate::intent::Intent;
use crate::mark_view::mark_view;
use crate::metrics::SWITCHER_HEIGHT;
use crate::project::Project;
use crate::row_action::RowAction;
use crate::scale::{SPACE_LG, SPACE_MD, SPACE_XS};
use crate::sidebar::Sidebar;
use crate::tile_shape::TileShape;

const TILE_PULL: f32 = -4.0;

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
        let family = Theme::of(cx).heading_family();
        self.sync_space_icon(cx);
        let editor = self.offers_space_icon().then(|| self.icon_editor.clone());
        let block = active_block(
            current,
            (Self::add_button(cx), editor),
            (reveal, family),
            palette,
        );
        Some(
            div()
                .id("switcher")
                .flex()
                .flex_none()
                .items_center()
                .gap(SPACE_MD)
                .h(px(SWITCHER_HEIGHT))
                .px(SPACE_LG)
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

fn mark_tile(
    project: &Project,
    group: &SharedString,
    palette: &Palette,
) -> Div {
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

fn name_cell(
    current: &Project,
    family: &'static str,
    palette: &Palette,
) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .font_family(family)
        .text_size(px(NAME_SIZE))
        .line_height(px(NAME_LINE))
        .font_weight(FontWeight::NORMAL)
        .text_color(palette.foreground())
        .child(current.name.clone())
}

fn editable_tile(
    tile: Div,
    mark: AnyElement,
    editor: Option<Entity<IconEditor>>,
) -> AnyElement {
    let Some(editor) = editor else {
        return tile.child(mark).into_any_element();
    };
    let marked = div()
        .relative()
        .size(px(MARK))
        .child(mark)
        .child(edit_pencil(px(MARK)));
    Popover::new(
        "space-icon",
        tile.group(GROUP).child(marked),
        move |_, _| editor.clone(),
    )
    .placement(Placement::Right)
    .into_any_element()
}

fn active_block(
    current: &Project,
    (add, editor): (IconButton, Option<Entity<IconEditor>>),
    (reveal, family): (f32, &'static str),
    palette: &Palette,
) -> Div {
    let mark = mark_view(&current.mark, px(MARK), palette);
    let tile = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(TILE))
        .ml(px(TILE_PULL))
        .rounded(radius::MD);
    let tile = editable_tile(tile, mark, editor);
    let name = name_cell(current, family, palette);
    let slot = div()
        .flex_none()
        .overflow_hidden()
        .w(px(TILE * reveal))
        .ml(px(TILE_PULL * (1.0 - reveal)))
        .opacity(reveal)
        .child(add);
    div()
        .flex()
        .flex_1()
        .min_w_0()
        .items_center()
        .gap(SPACE_XS)
        .child(tile)
        .child(name)
        .child(slot)
}

#[cfg(test)]
mod tests {
    use crate::intent::Intent;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn several_spaces_render_the_other_marks(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        cx.run_until_parked();
        assert!(sidebar.read_with(cx, |this, _| this.data.projects().len() > 1));
    }

    #[gpui::test]
    fn the_add_button_starts_the_create_space_flow(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::StartCreateSpace, window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.space.is_some()));
    }
}
