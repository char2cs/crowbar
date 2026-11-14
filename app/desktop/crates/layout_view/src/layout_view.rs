use std::rc::Rc;
use std::time::Instant;

use gpui::{App, Bounds, Context, EventEmitter, Pixels};
use model::{Rect, Tab};
use protocol::PaneId;
use ui::{Motion, OverlayGuard};

use crate::chrome_context::ChromeContext;
use crate::drop_hover::DropHover;
use crate::layout_event::LayoutEvent;
use crate::pane_fades::PaneFades;
use crate::pane_source::PaneSource;
use crate::sash_grab::SashGrab;

/// The active tab's panes: their frames, the sashes between them, the accent ring, the pane
/// handle and the drop preview. It owns no layout state: it renders the [`Tab`] it was last
/// given and answers pointer input with [`LayoutEvent::Apply`] commands.
pub struct LayoutView {
    pub(crate) tab: Option<Tab>,
    pub(crate) chrome: ChromeContext,
    pub(crate) source: Rc<dyn PaneSource>,
    pub(crate) bounds: Bounds<Pixels>,
    pub(crate) hovered: Option<PaneId>,
    pub(crate) sash: Option<SashGrab>,
    pub(crate) drop: DropHover,
    pub(crate) rings: PaneFades,
    pub(crate) handles: PaneFades,
    pub(crate) handle_overlay: Option<OverlayGuard>,
}

impl std::fmt::Debug for LayoutView {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("LayoutView")
            .field("tab", &self.tab.as_ref().map(Tab::id))
            .field("hovered", &self.hovered)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<LayoutEvent> for LayoutView {}

impl LayoutView {
    /// A view of no tab yet; `source` supplies pane content, labels and new tab ids.
    pub fn new(source: Rc<dyn PaneSource>) -> Self {
        Self {
            tab: None,
            chrome: ChromeContext::default(),
            source,
            bounds: Bounds::default(),
            hovered: None,
            sash: None,
            drop: DropHover::default(),
            rings: PaneFades::default(),
            handles: PaneFades::default(),
            handle_overlay: None,
        }
    }

    /// Shows `tab`, the model's active tab, or nothing when no tab is open.
    pub fn set_tab(&mut self, tab: Option<Tab>, cx: &mut Context<'_, Self>) {
        self.tab = tab;
        let panes = self.tab.as_ref().map_or_else(Vec::new, Tab::pane_ids);
        self.rings.retain(&panes);
        self.handles.retain(&panes);
        self.sync_fades(cx);
        cx.notify();
    }

    /// Tells the view what surrounds it so the outer panes get the right borders.
    pub fn set_chrome(&mut self, chrome: ChromeContext, cx: &mut Context<'_, Self>) {
        self.chrome = chrome;
        cx.notify();
    }

    /// The pane the pointer is over, when the shell learns it from a web view instead of from
    /// this view's own hover tracking.
    pub fn set_hovered_pane(&mut self, pane: Option<PaneId>, cx: &mut Context<'_, Self>) {
        self.hovered = pane;
        self.sync_fades(cx);
        cx.notify();
    }

    pub(crate) fn local(&self) -> Rect {
        Rect {
            x: 0.0,
            y: 0.0,
            width: f32::from(self.bounds.size.width),
            height: f32::from(self.bounds.size.height),
        }
    }

    pub(crate) fn measured(&mut self, bounds: Bounds<Pixels>, cx: &mut Context<'_, Self>) {
        if self.bounds != bounds {
            self.bounds = bounds;
            cx.notify();
        }
    }

    pub(crate) fn sync_fades(&mut self, cx: &mut App) {
        let (now, motion) = (Instant::now(), Motion::of(cx));
        let multiple = self.tab.as_ref().is_some_and(|tab| tab.len() > 1);
        let focused = self.tab.as_ref().and_then(|tab| tab.focused().cloned());
        let ring = focused.filter(|_| multiple);
        if let Some(pane) = &ring {
            self.rings.show(pane, now, motion);
        }
        self.rings.hide_all_but(ring.as_ref(), now, motion);
        let handle = self.hovered.clone().filter(|_| multiple);
        if let Some(pane) = &handle {
            self.handles.show(pane, now, motion);
        }
        self.handles.hide_all_but(handle.as_ref(), now, motion);
    }
}

#[cfg(test)]
#[path = "layout_view_tests.rs"]
mod tests;
