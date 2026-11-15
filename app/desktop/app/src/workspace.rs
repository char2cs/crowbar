use gpui::{
    Bounds, Context, Entity, EventEmitter, IntoElement, ParentElement, Pixels, Render, Styled,
    Window, canvas, div,
};
use layout_view::LayoutView;
use tab_bar::TabBar;

use crate::workspace_event::WorkspaceEvent;

pub(crate) struct Workspace {
    tabs: Entity<TabBar>,
    layout: Entity<LayoutView>,
    area: Option<Bounds<Pixels>>,
}

impl EventEmitter<WorkspaceEvent> for Workspace {}

impl Workspace {
    pub(crate) fn new(
        tabs: Entity<TabBar>,
        layout: Entity<LayoutView>,
    ) -> Self {
        Self {
            tabs,
            layout,
            area: None,
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
}

impl Render for Workspace {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let this = cx.entity();
        div()
            .size_full()
            .flex()
            .flex_col()
            .child(self.tabs.clone())
            .child(
                div().flex_1().relative().child(self.layout.clone()).child(
                    canvas(
                        move |bounds, _, cx| {
                            this.update(cx, |workspace, cx| workspace.measured(bounds, cx));
                        },
                        |_, (), _, _| {},
                    )
                    .absolute()
                    .inset_0()
                    .size_full(),
                ),
            )
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{AppContext as _, TestAppContext, px, size};
    use model::{Layout, TabId};

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
            idle_terminals(TerminalIds::default()),
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
            Workspace::new(tabs, layout)
        });
        let areas = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&areas);
        cx.update(|_, cx| cx.subscribe(&workspace, record_into(sink)).detach());
        cx.simulate_resize(size(px(900.0), px(700.0)));
        cx.run_until_parked();
        cx.update(|window, cx| window.draw(cx).clear());
        let measured = areas.borrow().last().copied().unwrap();
        assert!(
            measured.size.width > px(0.0) && measured.size.height > px(0.0),
            "{measured:?}"
        );
        let count = areas.borrow().len();
        cx.update(|window, cx| window.draw(cx).clear());
        assert_eq!(
            areas.borrow().len(),
            count,
            "an unchanged area is not reported again"
        );
    }
}
