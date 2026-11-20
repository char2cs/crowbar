use std::rc::Rc;

use gpui::{
    AppContext as _, Context, Entity, IntoElement, ParentElement, Render, Styled, Subscription,
    Window, div, px,
};
use layout::{Layout, LayoutHistory, PaneInstance};
use layoutview::{ChromeContext, LayoutEvent, LayoutView, SidebarEdge};
use ui::{Surface, Theme};

use crate::chip::chip;
use crate::demo_source::DemoSource;

const SIDEBAR: f32 = 296.0;
const SIDEBAR_PADDING: f32 = 12.0;
const SIDEBAR_GAP: f32 = 8.0;
const STRIP: f32 = 42.0;

pub(crate) struct Demo {
    history: LayoutHistory,
    view: Entity<LayoutView>,
    chips: Vec<PaneInstance>,
    _events: Subscription,
}

impl Demo {
    pub(crate) fn new(
        (layout, chips): (Layout, Vec<PaneInstance>),
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let view = cx.new(|_| LayoutView::new(Rc::new(DemoSource::starting_at_first_tab())));
        let chrome = ChromeContext {
            sidebar: SidebarEdge::Left,
            side: layout::SidebarSide::Left,
            tab_strip: true,
        };
        let tab = layout.active_tab().cloned();
        view.update(cx, |view, cx| {
            view.set_chrome(chrome, cx);
            view.set_tab(tab, cx);
        });
        let events = cx.subscribe(&view, Self::on_event);
        Self {
            history: LayoutHistory::new(layout),
            view,
            chips,
            _events: events,
        }
    }
}

impl Demo {
    fn on_event(
        &mut self,
        _: Entity<LayoutView>,
        event: &LayoutEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let LayoutEvent::Apply(command) = event else {
            return;
        };
        if self.history.apply(command.clone()).is_ok() {
            let tab = self.history.layout().active_tab().cloned();
            self.view.update(cx, |view, cx| view.set_tab(tab, cx));
        }
    }
}

impl Render for Demo {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        div()
            .size_full()
            .flex()
            .bg(palette.chrome_bg())
            .child(
                div()
                    .w(px(SIDEBAR))
                    .h_full()
                    .p(px(SIDEBAR_PADDING))
                    .flex()
                    .flex_col()
                    .gap(px(SIDEBAR_GAP))
                    .children(self.chips.iter().map(|instance| chip(instance, cx))),
            )
            .child(
                div()
                    .flex_1()
                    .flex()
                    .flex_col()
                    .child(div().h(px(STRIP)))
                    .child(div().flex_1().child(self.view.clone())),
            )
    }
}
