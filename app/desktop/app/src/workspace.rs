use std::rc::Rc;

use gpui::{
    AnyElement, Bounds, Context, Entity, EventEmitter, InteractiveElement as _, IntoElement,
    ParentElement, Pixels, Render, Styled, Window, canvas, div, px,
};
use layoutview::LayoutView;
use viewtabs::TabBar;

use crate::backend::ProjectsFailed;
use crate::workspace_event::WorkspaceEvent;

// SPEC: docs/triage/2026-11-18-side-by-side-audit/01-oobe.md OOBE-009: develop's InlineError is flex-1 p-6 under the WorkspaceHost.
const LANDING_PADDING: f32 = 24.0;
const STAGE_SELECTOR: &str = "workspace-stage";

pub(crate) type ProjectsFailing = Rc<dyn Fn() -> bool>;

pub(crate) struct Workspace {
    tabs: Entity<TabBar>,
    layout: Entity<LayoutView>,
    area: Option<Bounds<Pixels>>,
    landing: Entity<ProjectsFailed>,
    failing: ProjectsFailing,
}

impl EventEmitter<WorkspaceEvent> for Workspace {}

impl Workspace {
    pub(crate) fn new(
        (tabs, layout): (Entity<TabBar>, Entity<LayoutView>),
        landing: Entity<ProjectsFailed>,
        failing: ProjectsFailing,
    ) -> Self {
        Self {
            tabs,
            layout,
            area: None,
            landing,
            failing,
        }
    }

    fn measured(
        &mut self,
        bounds: Bounds<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        if self.area != Some(bounds) {
            self.area = Some(bounds);
            cx.emit(WorkspaceEvent::Measured(bounds));
        }
    }

    fn stage(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> gpui::Div {
        let this = cx.entity();
        let stage = div()
            .relative()
            .flex_1()
            .debug_selector(|| STAGE_SELECTOR.into());
        stage.child(self.layout.clone()).child(
            canvas(
                move |bounds, _, cx| {
                    this.update(cx, |workspace, cx| workspace.measured(bounds, cx));
                },
                |_, (), _, _| {},
            )
            .absolute()
            .inset_0()
            .size_full(),
        )
    }

    fn landing(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        if !(self.failing)() {
            return None;
        }
        let landing = self.landing.clone();
        if !self.tabs.read(cx).has_tabs() {
            return Some(div().absolute().inset_0().child(landing).into_any_element());
        }
        let below = div().relative().flex_1().py(px(LANDING_PADDING));
        Some(below.child(landing).into_any_element())
    }
}

impl Render for Workspace {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        div()
            .size_full()
            .flex()
            .flex_col()
            .child(self.tabs.clone())
            .child(
                div()
                    .flex_1()
                    .relative()
                    .flex()
                    .flex_col()
                    .child(self.stage(cx))
                    .children(self.landing(cx)),
            )
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{AppContext as _, TestAppContext, px, size};
    use layout::{Layout, TabId};

    use super::*;
    use crate::browser_bars::BrowserBars;
    use crate::browser_ids::BrowserIds;
    use crate::serials::Serials;
    use crate::shell_pane_source::ShellPaneSource;
    use crate::terminal_ids::TerminalIds;
    use crate::test_support::idle_terminals;
    use crate::titles::Titles;

    fn source() -> Rc<ShellPaneSource> {
        let bars = BrowserBars::new(BrowserIds::default(), async_channel::unbounded().0);
        Rc::new(ShellPaneSource::new(
            Titles::default(),
            Serials::above(&Layout::default()),
            bars,
            crate::native_panes::NativePanes::new(
                idle_terminals(TerminalIds::default()),
                crate::new_tab_panes::NewTabPanes::new(async_channel::unbounded().0),
                crate::test_support::idle_editors(),
            ),
        ))
    }

    fn record_into(
        sink: Rc<RefCell<Vec<Bounds<Pixels>>>>
    ) -> impl FnMut(Entity<Workspace>, &WorkspaceEvent, &mut gpui::App) {
        move |_, event, _| {
            let WorkspaceEvent::Measured(bounds) = event;
            sink.borrow_mut().push(*bounds);
        }
    }

    #[gpui::test]
    fn the_area_under_the_tab_strip_is_reported_when_it_changes(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap_or_else(|error| panic!("{error}")));
        let (workspace, cx) = cx.add_window_view(|_, cx| {
            let layout = cx.new(|_| LayoutView::new(source()));
            let tabs = cx.new(|_| TabBar::new(TabId(1)));
            let landing = cx.new(|_| ProjectsFailed::new(Rc::new(|| {})));
            Workspace::new((tabs, layout), landing, Rc::new(|| false))
        });
        let areas = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&areas);
        cx.update(|_, cx| cx.subscribe(&workspace, record_into(sink)).detach());
        cx.simulate_resize(size(px(900.0), px(700.0)));
        cx.run_until_parked();
        cx.update(|window, cx| window.draw(cx).clear(cx));
        let measured = areas.borrow().last().copied().unwrap();
        assert!(
            measured.size.width > px(0.0) && measured.size.height > px(0.0),
            "{measured:?}"
        );
        let count = areas.borrow().len();
        cx.update(|window, cx| window.draw(cx).clear(cx));
        assert_eq!(
            areas.borrow().len(),
            count,
            "an unchanged area is not reported again"
        );
    }
}
