//! The chats panel: one scrolling tree per space on a horizontal carousel.

mod foot_menu;
mod kinds;
mod parts;
mod reorder;
mod row_menu;
mod tree_rows;

pub(crate) use foot_menu::foot_menu;
pub use parts::{ChatGlyph, GlyphTone};
pub(crate) use parts::{RowBounds, RowInput, Trailing, input_view, ringed, row_focus};
pub(crate) use reorder::DragState;
pub(crate) use reorder::{GhostBuild, RowGhost};
pub(crate) use row_menu::row_menu;

use std::rc::Rc;

use gpui::{
    AnyElement, Context, DragMoveEvent, InteractiveElement, IntoElement, ParentElement,
    SharedString, Styled, Window, div, px,
};
use sidebar::{EdgeScroll, HostTone, Notice, Project, ProjectTree, Row, ScopeSync};
use ui::{DraggedRow, Motion, ScrollArea};

use crate::panels::chats::kinds::notice_row::NoticeRow;
use crate::panels::chats::kinds::skeleton_rows::skeleton_rows;
use crate::panels::chats::kinds::tree_foot::TreeFoot;
use crate::sidebar::SPACE_MD;
use crate::sidebar::Sidebar;
use crate::sidebar::{RowAction, RowHandler};

const SKELETON_MARGIN: f32 = 24.0;

fn count_as_f32(count: usize) -> f32 {
    f32::from(u16::try_from(count).unwrap_or(u16::MAX))
}

fn notice_view(
    notice: Notice,
    project: &Project,
    handler: &RowHandler,
) -> AnyElement {
    NoticeRow {
        notice,
        project: project.id.clone(),
        handler: Rc::clone(handler),
    }
    .into_any_element()
}

impl Sidebar {
    pub(crate) fn row_handler(cx: &mut Context<'_, Self>) -> RowHandler {
        let entity = cx.entity();
        Rc::new(move |action, window, cx| {
            entity.update(cx, |this, cx| this.act(action, window, cx));
        })
    }

    pub(crate) fn chats_panel(
        &mut self,
        width: f32,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        if self.waits_for_spaces() {
            return skeleton_rows(width - SKELETON_MARGIN).into_any_element();
        }
        let projects = self.data.projects();
        let trees: Vec<ProjectTree> = projects
            .iter()
            .map(|project| self.data.tree(&project.id))
            .collect();
        let rows: Vec<Row> = trees.iter().flat_map(|tree| tree.rows.clone()).collect();
        self.tree.reconcile(&rows);
        self.scroll_edge(window);
        let offset = self.carousel_offset((&projects, width), window, cx);
        let panels: Vec<AnyElement> = projects
            .iter()
            .zip(trees)
            .map(|(project, tree)| self.space_panel((project, tree), width, cx))
            .collect();
        let strip = div()
            .flex()
            .h_full()
            .w(px(width * count_as_f32(projects.len().max(1))))
            .ml(px(-offset))
            .children(panels);
        div()
            .size_full()
            .overflow_hidden()
            .child(strip)
            .into_any_element()
    }

    fn waits_for_spaces(&self) -> bool {
        if self.data.sync() == ScopeSync::Ready {
            return false;
        }
        let shown = self.data.shown_host();
        let hosts = self.data.hosts();
        let host = hosts.iter().find(|host| Some(&host.id) == shown.as_ref());
        !host.is_some_and(|host| host.state.tone() == HostTone::Offline)
    }

    fn carousel_offset(
        &mut self,
        (projects, width): (&[Project], f32),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> f32 {
        let active = self.data.active_project();
        let index = projects
            .iter()
            .position(|project| Some(&project.id) == active.as_ref());
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        self.carousel
            .go_to(count_as_f32(index.unwrap_or(0)), now, motion);
        if self.carousel.is_running(now, motion) || self.tree.removals().tray().is_counting() {
            window.request_animation_frame();
        }
        self.carousel.value(now, motion) * width
    }

    fn space_panel(
        &mut self,
        (project, tree): (&Project, ProjectTree),
        width: f32,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let handler = Self::row_handler(cx);
        let notice = tree
            .notice
            .clone()
            .map(|notice| notice_view(notice, project, &handler));
        let foot = TreeFoot {
            project: project.id.clone(),
            handler: Rc::clone(&handler),
        };
        let me = cx.weak_entity();
        let rows = self.tree_rows(&tree, (&handler, width, &me));
        let list = div()
            .flex()
            .flex_col()
            .pb(SPACE_MD)
            .children(notice)
            .children(rows)
            .child(foot);
        let handle = self.scrolls.of_space(&project.id);
        let edge_handler = Rc::clone(&handler);
        let id = SharedString::from(format!("space-scroll-{}", project.id.as_str()));
        let lock = self.drag.lifted.is_some().then(wheel_lock);
        div()
            .relative()
            .flex_none()
            .w(px(width))
            .h_full()
            .on_drag_move(move |event: &DragMoveEvent<DraggedRow>, window, cx| {
                let (top, bottom) = (event.bounds.top(), event.bounds.bottom());
                let pull = EdgeScroll::between((top.into(), bottom.into()))
                    .pull(event.event.position.y.into());
                edge_handler(RowAction::Edge(pull), window, cx);
            })
            .child(ScrollArea::new(id, &handle, list))
            .children(lock)
            .into_any_element()
    }
}

fn wheel_lock() -> AnyElement {
    div()
        .absolute()
        .inset_0()
        .on_scroll(|_, _, cx| cx.stop_propagation())
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::blank::Blank;
    use crate::test_support::drag::lift_a_row;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_row_handler_forwards_actions_to_the_sidebar(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoShell::reference());
        let handler = sidebar.update(cx, |_, cx| Sidebar::row_handler(cx));
        cx.update(|window, cx| handler(RowAction::Open(RowId::from("chat")), window, cx));
        cx.run_until_parked();
        assert_eq!(recorder.sent(), vec![Command::OpenRow(RowId::from("chat"))]);
    }

    type Setup = fn(&mut Sidebar, &mut Context<'_, Sidebar>);

    fn untouched(
        _: &mut Sidebar,
        _: &mut Context<'_, Sidebar>,
    ) {
    }

    fn scrolled_by_the_wheel(
        cx: &mut TestAppContext,
        setup: Setup,
    ) -> f32 {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        cx.simulate_resize(gpui::size(px(1200.0), px(320.0)));
        sidebar.update(cx, setup);
        cx.run_until_parked();
        cx.simulate_event(gpui::ScrollEvent {
            position: point(px(100.0), px(240.0)),
            delta: gpui::ScrollDelta::Pixels(point(px(0.0), px(-60.0))),
            ..gpui::ScrollEvent::default()
        });
        cx.run_until_parked();
        sidebar.update(cx, |this, _| {
            let project = this.data.active_project().expect("a space");
            -f32::from(this.scrolls.of_space(&project).offset().y)
        })
    }

    #[gpui::test]
    fn the_wheel_scrolls_the_tree(cx: &mut TestAppContext) {
        assert!(scrolled_by_the_wheel(cx, untouched) > 0.0);
    }

    #[gpui::test]
    fn the_wheel_leaves_the_tree_still_while_a_row_is_dragged(cx: &mut TestAppContext) {
        assert_eq!(scrolled_by_the_wheel(cx, lift_a_row), 0.0);
    }

    const FRAMES: usize = 3;
    const PULL: f32 = 10.0;

    #[gpui::test]
    fn a_row_held_at_the_edge_keeps_scrolling_the_tree_every_frame(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        cx.simulate_resize(gpui::size(px(1200.0), px(320.0)));
        sidebar.update(cx, lift_a_row);
        cx.run_until_parked();
        sidebar.update_in(cx, |this, window, cx| {
            this.act(RowAction::Edge(PULL), window, cx);
        });
        for _ in 0..FRAMES {
            cx.update(Window::simulate_next_frame);
            cx.run_until_parked();
        }
        let scrolled = sidebar.update(cx, |this, _| {
            let project = this.data.active_project().expect("a space");
            -f32::from(this.scrolls.of_space(&project).offset().y)
        });
        assert!(scrolled >= PULL * 2.0, "scrolled {scrolled}");
    }

    fn panes_renders(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> usize {
        sidebar.read_with(cx, |this, cx| {
            let blank = this.content.clone().downcast::<Blank>().ok();
            blank.map_or(0, |blank| blank.read(cx).renders)
        })
    }

    #[gpui::test]
    fn an_edge_scroll_frame_leaves_the_panes_area_as_it_was(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        cx.simulate_resize(gpui::size(px(1200.0), px(320.0)));
        sidebar.update(cx, lift_a_row);
        sidebar.update_in(cx, |this, window, cx| {
            this.act(RowAction::Edge(PULL), window, cx);
        });
        cx.run_until_parked();
        let before = panes_renders(&sidebar, cx);
        assert!(before > 0, "the panes area was drawn");
        for _ in 0..FRAMES {
            cx.update(Window::simulate_next_frame);
            cx.run_until_parked();
        }
        assert_eq!(panes_renders(&sidebar, cx), before);
    }

    fn shows_skeleton(
        cx: &mut TestAppContext,
        data: DemoShell,
    ) -> bool {
        let (_, _, cx) = open(cx, data);
        cx.run_until_parked();
        cx.debug_bounds(kinds::skeleton_rows::SELECTOR).is_some()
    }

    #[gpui::test]
    fn a_shown_host_that_is_down_holds_no_skeleton(cx: &mut TestAppContext) {
        let waiting = shows_skeleton(cx, DemoShell::loading());
        let down = shows_skeleton(cx, DemoShell::showing_down_host());
        assert_eq!(
            (waiting, down),
            (true, false),
            "(waiting for a host, its host down)"
        );
    }
}
