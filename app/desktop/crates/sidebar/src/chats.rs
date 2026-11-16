//! The chats panel: one scrolling tree per space on a horizontal carousel.

use std::rc::Rc;
use std::time::Instant;

use gpui::{
    AnyElement, Context, DragMoveEvent, InteractiveElement, IntoElement, ParentElement,
    SharedString, Styled, Window, div, px,
};
use ui::{DraggedRow, Motion, ScrollArea};

use crate::edge_scroll::pull;
use crate::notice::Notice;
use crate::notice_row::NoticeRow;
use crate::project::Project;
use crate::project_tree::ProjectTree;
use crate::row::Row;
use crate::row_action::{RowAction, RowHandler};
use crate::scale::SPACE_MD;
use crate::scope_sync::ScopeSync;
use crate::sidebar::Sidebar;
use crate::skeleton_rows::skeleton_rows;
use crate::tree_foot::TreeFoot;

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
        if self.data.sync() == ScopeSync::Pending {
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
        let (now, motion) = (Instant::now(), Motion::of(cx));
        self.carousel
            .go_to(count_as_f32(index.unwrap_or(0)), now, motion);
        if self.carousel.is_running(now, motion) || self.tree.removals.tray().is_counting() {
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
        let rows = self.tree_rows(&tree, (&handler, width));
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
        div()
            .flex_none()
            .w(px(width))
            .h_full()
            .on_drag_move(move |event: &DragMoveEvent<DraggedRow>, window, cx| {
                let (top, bottom) = (event.bounds.top(), event.bounds.bottom());
                let pull = pull((top.into(), bottom.into()), event.event.position.y.into());
                edge_handler(RowAction::Edge(pull), window, cx);
            })
            .child(ScrollArea::new(id, &handle, list))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_row_handler_forwards_actions_to_the_sidebar(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        let handler = sidebar.update(cx, |_, cx| Sidebar::row_handler(cx));
        cx.update(|window, cx| handler(RowAction::Open(RowId::from("main")), window, cx));
        cx.run_until_parked();
        assert_eq!(recorder.sent(), vec![Command::OpenRow(RowId::from("main"))]);
    }

    #[gpui::test]
    fn selecting_a_space_makes_it_the_active_one(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        let other = sidebar.read_with(cx, |this, _| {
            let active = this.data.active_project();
            this.data
                .projects()
                .into_iter()
                .map(|project| project.id)
                .find(|id| Some(id) != active.as_ref())
        });
        let other = other.expect("a second space");
        sidebar.update(cx, |this, cx| {
            this.send(Command::SelectProject(other.clone()), cx);
        });
        cx.run_until_parked();
        assert_eq!(
            sidebar.read_with(cx, |this, _| this.data.active_project()),
            Some(other)
        );
    }

    #[gpui::test]
    fn the_loading_state_renders_a_skeleton_instead_of_rows(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::loading());
        cx.run_until_parked();
        assert_eq!(sidebar.read_with(cx, |this, _| this.mode), Mode::Chats);
    }
}
