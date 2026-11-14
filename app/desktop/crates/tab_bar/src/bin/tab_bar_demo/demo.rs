use gpui::{
    AppContext as _, Context, Entity, IntoElement, ParentElement, Render, Styled, Subscription,
    Window, div, px,
};
use model::LayoutHistory;
use tab_bar::{SidebarState, TabBar, TabBarEvent, TabView};
use ui::{Surface, Theme};

use crate::chip::chip;
use crate::scenario;

const SIDEBAR: f32 = 296.0;
const CHIPS: [&str; 2] = ["delta", "epsilon"];

pub(crate) struct Demo {
    history: LayoutHistory,
    bar: Entity<TabBar>,
    closed: bool,
    _events: Subscription,
}

fn views(history: &LayoutHistory) -> (Vec<TabView>, Option<model::TabId>) {
    let layout = history.layout();
    let tabs = layout
        .tabs()
        .iter()
        .map(|tab| TabView::from_tab(tab, scenario::member))
        .collect();
    (tabs, layout.active_tab().map(model::Tab::id))
}

impl Demo {
    pub(crate) fn new(name: &str, cx: &mut Context<'_, Self>) -> Self {
        let history = LayoutHistory::new(scenario::build(name));
        let bar = cx.new(|_| TabBar::new(model::TabId(100)));
        let closed = name == "closed";
        let (tabs, active) = views(&history);
        let sidebar = if closed {
            SidebarState::ClosedLeft
        } else {
            SidebarState::Open
        };
        bar.update(cx, |bar, cx| {
            bar.set_tabs(tabs, active, cx);
            bar.set_sidebar(sidebar, cx);
        });
        let events = cx.subscribe(&bar, Self::on_event);
        Self {
            history,
            bar,
            closed,
            _events: events,
        }
    }
}

impl Demo {
    fn on_event(&mut self, _: Entity<TabBar>, event: &TabBarEvent, cx: &mut Context<'_, Self>) {
        let TabBarEvent::Apply(commands) = event else {
            return;
        };
        let applied = commands
            .iter()
            .try_for_each(|command| self.history.apply(command.clone()).map(drop));
        let (tabs, active) = views(&self.history);
        self.bar
            .update(cx, |bar, cx| bar.set_tabs(tabs, active, cx));
        drop(applied);
    }
}

impl Render for Demo {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let sidebar = div()
            .w(px(SIDEBAR))
            .h_full()
            .p(px(12.0))
            .flex()
            .flex_col()
            .gap(px(8.0));
        let sidebar = if self.closed {
            div().w(px(0.0))
        } else {
            sidebar.children(CHIPS.map(|name| chip(name, cx)))
        };
        div()
            .size_full()
            .flex()
            .bg(palette.chrome_bg())
            .child(sidebar)
            .child(div().flex_1().child(self.bar.clone()))
    }
}
