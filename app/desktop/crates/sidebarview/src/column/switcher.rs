//! The 44px space switcher: active space name, add button and the other spaces.

mod edit_pencil;
mod hover_glide;
pub(crate) mod icon_editor;
mod space_icon_editor;

pub(crate) use icon_editor::IconEditor;

use gpui::{
    AnyElement, Context, Div, ElementId, Entity, FontWeight, InteractiveElement, IntoElement,
    ParentElement, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use sidebar::{Command, Intent, Project};
use ui::{HoverFill, Palette, Placement, Popover, Surface, Theme, Tooltip, radius, tracking};

use crate::column::switcher::edit_pencil::{GROUP, edit_pencil};

use crate::column::header::IconButton;
use crate::column::header::TileShape;
use crate::column::mark_view::mark_view;
use crate::sidebar::RowAction;
use crate::sidebar::Sidebar;
use crate::sidebar::{SPACE_LG, SPACE_MD, SPACE_XS, SWITCHER_HEIGHT};

const TILE_PULL: f32 = -4.0;

const TILE: f32 = 28.0;
const MARK: f32 = 20.0;
const NAME_SIZE: f32 = 19.0;
const NAME_LINE: f32 = 24.0;
const OTHER_MARK_ALPHA: f32 = 0.8;
const SELECTOR: &str = "sidebar-switcher";

impl Sidebar {
    pub(crate) fn switcher(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let projects = self.data.projects();
        let active = self.data.active_project();
        let current = projects
            .iter()
            .find(|project| Some(&project.id) == active.as_ref());
        let glide = hover_glide::track("switcher", &self.clock, window, cx);
        let reveal = match projects.len() {
            0 | 1 => 1.0,
            _ => hover_glide::value(&glide, window, cx),
        };
        let others: Vec<AnyElement> = projects
            .iter()
            .filter(|project| Some(&project.id) != current.map(|current| &current.id))
            .map(|project| Self::other_mark(project, palette, window, cx))
            .collect();
        let slot = add_slot(Self::add_button(cx), reveal);
        let (block, trailing) = match current {
            Some(current) => (self.named_block(current, slot, cx), None),
            None => (div().flex_1().min_w_0(), Some(slot)),
        };
        div()
            .id("switcher")
            .debug_selector(|| SELECTOR.into())
            .flex()
            .flex_none()
            .items_center()
            .gap(SPACE_MD)
            .h(px(SWITCHER_HEIGHT))
            .px(SPACE_LG)
            .overflow_hidden()
            .on_hover(hover_glide::listener(&glide))
            .child(block)
            .children(trailing)
            .children(others)
            .into_any_element()
    }

    fn named_block(
        &mut self,
        current: &Project,
        slot: Div,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let theme = *Theme::of(cx);
        self.sync_space_icon(cx);
        let editor = self.offers_space_icon().then(|| self.icon_editor.clone());
        active_block(current, (slot, editor), theme)
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
        let tile = HoverFill::new(ElementId::from(group.clone()), fill)
            .apply(mark_tile(project, &group, palette), (window, &mut **cx));
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
        .letter_spacing(tracking::at(tracking::TIGHT, px(NAME_SIZE)))
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

fn add_slot(
    add: IconButton,
    reveal: f32,
) -> Div {
    div()
        .flex_none()
        .overflow_hidden()
        .w(px(TILE * reveal))
        .ml(px(TILE_PULL * (1.0 - reveal)))
        .opacity(reveal)
        .child(add)
}

fn active_block(
    current: &Project,
    (slot, editor): (Div, Option<Entity<IconEditor>>),
    theme: Theme,
) -> Div {
    let (family, palette) = (theme.heading_family(), theme.palette(Surface::Chrome));
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
    use gpui::{InteractiveElement as _, ParentElement as _, Styled as _, div};
    use sidebar::{HostId, Intent, Mark, Project};
    use ui::{Surface, Theme};

    use super::{NAME_SIZE, name_cell};
    use crate::test_support::mount;
    use crate::test_support::prelude::*;

    const SEED: &str = "Crowbar Seed";
    /// Tailwind `tracking-tight`, in em.
    const TIGHT: f32 = -0.025;

    fn name_width(
        cx: &mut TestAppContext,
        spacing: Option<f32>,
    ) -> f32 {
        let seed = Project {
            id: ProjectId::from("seed"),
            name: SEED.to_owned(),
            mark: Mark::Fallback,
            host: HostId::from("local"),
        };
        let window = mount(cx, move |_, cx| {
            let theme = Theme::of(cx);
            let cell = name_cell(
                &seed,
                theme.heading_family(),
                theme.palette(Surface::Chrome),
            );
            let cell = match spacing {
                Some(spacing) => cell.letter_spacing(px(spacing)),
                None => cell,
            };
            div().flex().child(
                div()
                    .flex_none()
                    .debug_selector(|| "switcher-name".into())
                    .child(cell),
            )
        });
        let bounds = window.debug_bounds("switcher-name").unwrap_or_default();
        f32::from(bounds.size.width)
    }

    #[gpui::test]
    fn the_space_name_is_set_tight_as_develop_does(cx: &mut TestAppContext) {
        let tight = name_width(cx, None);
        let loose = name_width(cx, Some(0.0));
        let glyphs = f32::from(u16::try_from(SEED.chars().count()).expect("a short name"));
        let narrower = loose - tight;
        let expected = glyphs * -TIGHT * NAME_SIZE;
        assert!(
            (narrower - expected).abs() < 1.0,
            "{narrower} vs {expected}"
        );
    }

    #[gpui::test]
    fn an_empty_profile_still_offers_the_add_button(cx: &mut TestAppContext) {
        let (_, _, cx) = open(cx, DemoShell::empty());
        cx.run_until_parked();
        let row = cx.debug_bounds(super::SELECTOR).unwrap_or_default();
        assert_eq!(
            row.size.height,
            px(super::SWITCHER_HEIGHT),
            "develop keeps the switcher row with its + when no space exists"
        );
    }

    #[gpui::test]
    fn several_spaces_render_the_other_marks(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        cx.run_until_parked();
        assert!(sidebar.read_with(cx, |this, _| this.data.projects().len() > 1));
    }

    #[gpui::test]
    fn the_add_button_starts_the_create_space_flow(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        sidebar.update_in(cx, |this, window, cx| {
            this.apply(Intent::StartCreateSpace, window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.space.is_some()));
    }
}
