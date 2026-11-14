//! The chats panel: one scrolling tree per space on a horizontal carousel.

use std::rc::Rc;
use std::time::Instant;

use gpui::{
    AnyElement, Context, IntoElement, ParentElement, SharedString, Styled, Window, div, px,
};
use ui::{Motion, ScrollArea};

use crate::notice_row::NoticeRow;
use crate::project::Project;
use crate::project_tree::ProjectTree;
use crate::row::Row;
use crate::row_action::RowHandler;
use crate::scope_sync::ScopeSync;
use crate::sidebar::Sidebar;
use crate::skeleton_rows::skeleton_rows;
use crate::tree_foot::TreeFoot;

const SKELETON_MARGIN: f32 = 24.0;

fn count_as_f32(count: usize) -> f32 {
    f32::from(u16::try_from(count).unwrap_or(u16::MAX))
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
        if self.carousel.is_running(now, motion) || self.tree.tray().is_counting() {
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
        let notice = tree.notice.clone().map(|notice| {
            NoticeRow {
                notice,
                project: project.id.clone(),
                handler: Rc::clone(&handler),
            }
            .into_any_element()
        });
        let foot = TreeFoot {
            project: project.id.clone(),
            handler: Rc::clone(&handler),
        };
        let rows = self.tree_rows(&tree, &handler);
        let list = div()
            .flex()
            .flex_col()
            .pb(px(8.0))
            .children(notice)
            .children(rows)
            .child(foot);
        let handle = self.scroll_for(&project.id);
        let id = SharedString::from(format!("space-scroll-{}", project.id.as_str()));
        div()
            .flex_none()
            .w(px(width))
            .h_full()
            .child(ScrollArea::new(id, &handle, list))
            .into_any_element()
    }
}

#[cfg(test)]
#[path = "chats_tests.rs"]
mod tests;
