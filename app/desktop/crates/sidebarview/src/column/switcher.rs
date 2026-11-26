//! The 44px space switcher: active space name, add button and the other spaces.

pub(crate) mod edit_pencil;
mod host_line;
mod hover_glide;
pub(crate) mod icon_editor;
mod icon_label;
mod space_icon_editor;

pub(crate) use icon_editor::IconEditor;

use gpui::{
    AnyElement, Context, Div, ElementId, Entity, FontWeight, InteractiveElement, IntoElement,
    ParentElement, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use sidebar::{Command, Host, HostKind, Intent, Project};
use ui::{
    HoverFill, Palette, Placement, Popover, Surface, Theme, Tooltip, TrackedText, radius, tracking,
};

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
        let line = self
            .remote_host_of(current)
            .map(|host| Self::host_line(&host, &theme, cx));
        let tile = space_tile(current, editor, theme);
        let name = name_column(current, line, theme);
        active_block((tile, name, slot))
    }

    fn remote_host_of(
        &self,
        project: &Project,
    ) -> Option<Host> {
        let hosts = self.data.hosts();
        let host = hosts.into_iter().find(|host| host.id == project.host)?;
        (host.kind == HostKind::Remote).then_some(host)
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

fn spaced_name_cell(
    current: &Project,
    (family, em): (&'static str, f32),
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
        .child(TrackedText::new(current.name.clone(), em))
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
        .group(GROUP)
        .debug_selector(|| "space-mark".into())
        .child(mark)
        .child(edit_pencil(px(MARK)));
    let popover =
        Popover::new("space-icon", marked, move |_, _| editor.clone()).placement(Placement::Right);
    tile.child(popover).into_any_element()
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

fn space_tile(
    current: &Project,
    editor: Option<Entity<IconEditor>>,
    theme: Theme,
) -> AnyElement {
    let mark = mark_view(&current.mark, px(MARK), theme.palette(Surface::Chrome));
    let tile = div()
        .flex()
        .flex_none()
        .items_center()
        .justify_center()
        .size(px(TILE))
        .ml(px(TILE_PULL))
        .rounded(radius::MD);
    editable_tile(tile, mark, editor)
}

fn name_column(
    current: &Project,
    line: Option<AnyElement>,
    theme: Theme,
) -> Div {
    let (family, palette) = (theme.heading_family(), theme.palette(Surface::Chrome));
    div()
        .flex()
        .flex_col()
        .flex_1()
        .min_w_0()
        .child(spaced_name_cell(
            current,
            (family, tracking::TIGHT),
            palette,
        ))
        .children(line)
}

fn active_block((tile, name, slot): (AnyElement, Div, Div)) -> Div {
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

    use super::{NAME_SIZE, spaced_name_cell};
    use crate::test_support::mount;
    use crate::test_support::prelude::*;

    const SEED: &str = "Crowbar Seed";
    /// Tailwind `tracking-tight`, in em.
    const TIGHT: f32 = -0.025;

    fn name_width(
        cx: &mut TestAppContext,
        em: f32,
    ) -> f32 {
        let seed = Project {
            id: ProjectId::from("seed"),
            name: SEED.to_owned(),
            mark: Mark::Fallback,
            host: HostId::from("local"),
        };
        let window = mount(cx, move |_, cx| {
            let theme = Theme::of(cx);
            let cell = spaced_name_cell(
                &seed,
                (theme.heading_family(), em),
                theme.palette(Surface::Chrome),
            );
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
        let tight = name_width(cx, TIGHT);
        let loose = name_width(cx, 0.0);
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
    fn the_icon_popover_opens_level_with_the_mark_as_develops_align_start(cx: &mut TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        let mark = floating
            .window
            .debug_bounds("space-mark")
            .unwrap_or_default();
        floating
            .window
            .simulate_click(mark.center(), gpui::Modifiers::none());
        floating.window.run_until_parked();
        let editor = floating
            .window
            .debug_bounds("icon-editor")
            .unwrap_or_default();
        assert_eq!(
            f32::from(editor.top() - mark.top()),
            17.0,
            "the panel's border and padding sit above the editor, the panel's top on the mark's"
        );
    }

    fn shows_host_line(
        cx: &mut TestAppContext,
        data: DemoShell,
    ) -> bool {
        let (_, _, cx) = open(cx, data);
        cx.run_until_parked();
        cx.debug_bounds(super::host_line::SELECTOR).is_some()
    }

    #[gpui::test]
    fn only_a_remote_space_names_its_host_under_its_name(cx: &mut TestAppContext) {
        let remote = shows_host_line(cx, DemoShell::on_down_host());
        let local = shows_host_line(cx, DemoShell::rich());
        assert_eq!((remote, local), (true, false), "(remote, local)");
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
