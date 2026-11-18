use gpui::{
    App, AppContext as _, Context, Entity, IntoElement, ParentElement, Render, Styled,
    Subscription, Window, div, px,
};
use layout::{Layout, LayoutHistory, PaneInstance};
use ui::{Surface, Theme};
use viewtabs::{SidebarState, TabBar, TabBarEvent, TabView};

use crate::chip::chip;
use crate::scenario;

const CLOSED_SCENARIO: &str = "closed";
const SIDEBAR: f32 = 296.0;
const SIDEBAR_PADDING: f32 = 12.0;
const SIDEBAR_GAP: f32 = 8.0;

pub(crate) struct Demo {
    history: LayoutHistory,
    bar: Entity<TabBar>,
    chips: Vec<PaneInstance>,
    closed: bool,
    _events: Subscription,
}

fn views(history: &LayoutHistory) -> (Vec<TabView>, Option<layout::TabId>) {
    let layout = history.layout();
    let tabs = layout
        .tabs()
        .iter()
        .map(|tab| TabView::from_tab(tab, scenario::member))
        .collect();
    (tabs, layout.active_tab().map(layout::Tab::id))
}

fn sidebar_state(scenario: &str) -> SidebarState {
    if scenario == CLOSED_SCENARIO {
        return SidebarState::ClosedLeft;
    }
    SidebarState::Open
}

impl Demo {
    pub(crate) fn new(
        name: &str,
        (layout, chips): (Layout, Vec<PaneInstance>),
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let history = LayoutHistory::new(layout);
        let bar = cx.new(|_| TabBar::new(layout::TabId(100)));
        let closed = name == CLOSED_SCENARIO;
        let (tabs, active) = views(&history);
        let sidebar = sidebar_state(name);
        bar.update(cx, |bar, cx| {
            bar.set_tabs(tabs, active, cx);
            bar.set_sidebar(sidebar, cx);
        });
        let events = cx.subscribe(&bar, Self::on_event);
        Self {
            history,
            bar,
            chips,
            closed,
            _events: events,
        }
    }
}

impl Demo {
    fn on_event(
        &mut self,
        _: Entity<TabBar>,
        event: &TabBarEvent,
        cx: &mut Context<'_, Self>,
    ) {
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

impl Demo {
    fn sidebar(
        &self,
        cx: &App,
    ) -> gpui::Div {
        if self.closed {
            return div().w(px(0.0));
        }
        div()
            .w(px(SIDEBAR))
            .h_full()
            .p(px(SIDEBAR_PADDING))
            .flex()
            .flex_col()
            .gap(px(SIDEBAR_GAP))
            .children(self.chips.iter().map(|instance| chip(instance, cx)))
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
            .child(self.sidebar(cx))
            .child(div().flex_1().child(self.bar.clone()))
    }
}
