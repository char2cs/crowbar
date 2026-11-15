use gpui::{
    Bounds, Context, Entity, EventEmitter, IntoElement, ParentElement, Pixels, Render, Styled,
    Window, canvas, div,
};
use layout_view::LayoutView;
use tab_bar::TabBar;

/// What the workspace reports to the shell.
#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) enum WorkspaceEvent {
    Measured(Bounds<Pixels>),
}

/// The area beside the sidebar: the tab strip above the pane layout. It measures where the
/// layout sits in the window so the pane host can place web views in its holes.
pub(crate) struct Workspace {
    tabs: Entity<TabBar>,
    layout: Entity<LayoutView>,
    area: Option<Bounds<Pixels>>,
}

impl EventEmitter<WorkspaceEvent> for Workspace {}

impl Workspace {
    pub(crate) fn new(tabs: Entity<TabBar>, layout: Entity<LayoutView>) -> Self {
        Self {
            tabs,
            layout,
            area: None,
        }
    }

    fn measured(&mut self, bounds: Bounds<Pixels>, cx: &mut Context<'_, Self>) {
        if self.area != Some(bounds) {
            self.area = Some(bounds);
            cx.emit(WorkspaceEvent::Measured(bounds));
        }
    }
}

impl Render for Workspace {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
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
                    .size_full(),
                ),
            )
    }
}
