use std::rc::Rc;
use std::time::Instant;

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, Context, InteractiveElement, IntoElement, MouseButton, MouseDownEvent,
    ParentElement, Render, StatefulInteractiveElement, Styled, Window, canvas, deferred, div, px,
};
use layout::{Command, DragSubject, TabId};
use ui::{Motion, Surface, Theme, layer};

use crate::gesture::TabDrop;
use crate::pill::ShellLook;
use crate::pill::{CloseAction, TabShell};
use crate::strip_metrics::{
    ROW_GAP, ROW_HEIGHT, ROW_PADDING, SCROLLER_PADDING, SHELL_MARGIN, TAB_HEIGHT, TOGGLE_SIZE,
    TRAFFIC_LIGHT_INSET,
};
use crate::tab_bar::TabBar;
use crate::tab_bar::pointer::follow_pointer;
use crate::tab_bar::sidebar_glyph::SidebarGlyph;
use crate::tab_bar::sidebar_state::SidebarState;
use crate::tab_bar_event::TabBarEvent;

const DROP_LINE_INSET: f32 = 4.0;

impl Render for TabBar {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        if self.is_animating(now, motion) {
            window.request_animation_frame();
        }
        self.scroll_for_drag();
        let view = cx.entity();
        let pointer = view.clone();
        div()
            .id("tab-bar")
            .relative()
            .w_full()
            .h(px(ROW_HEIGHT))
            .child(
                canvas(
                    move |bounds, _, cx: &mut App| view.update(cx, |this, _| this.row = bounds),
                    move |_, (), window, cx| follow_pointer(&pointer, window, cx),
                )
                .absolute()
                .size_full(),
            )
            .child(self.row(now, motion, cx))
            .children(self.ghost())
            .children(self.drop_line(now, motion, cx))
            .children(self.menu_layers(window, cx))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::released))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::released))
            .on_drag_move::<DragSubject>(cx.listener(Self::drag_over))
            .on_drop::<DragSubject>(cx.listener(Self::dropped))
    }
}

impl TabBar {
    fn row(
        &self,
        now: Instant,
        motion: Motion,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        div()
            .flex()
            .items_center()
            .gap(px(ROW_GAP))
            .size_full()
            .px(px(ROW_PADDING))
            .when(self.sidebar == SidebarState::ClosedLeft, |row| {
                row.pl(px(ROW_PADDING + TRAFFIC_LIGHT_INSET))
                    .child(Self::toggle(SidebarState::ClosedLeft, cx))
            })
            .child(self.scroller(now, motion, cx))
            .when(self.sidebar == SidebarState::ClosedRight, |row| {
                row.child(Self::toggle(SidebarState::ClosedRight, cx))
            })
    }

    fn is_animating(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.drag
            .as_ref()
            .is_some_and(|drag| drag.is_sliding(now, motion))
            || self
                .settle
                .as_ref()
                .is_some_and(|settle| settle.is_running(now, motion))
            || self
                .incoming
                .as_ref()
                .is_some_and(|incoming| incoming.is_moving(now, motion))
    }

    fn scroller(
        &self,
        now: Instant,
        motion: Motion,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let spans = self.slot_spans();
        let slots: Vec<AnyElement> = self
            .tabs
            .iter()
            .enumerate()
            .zip(spans)
            .map(|((index, tab), span)| {
                let dx = self.shift(index, tab.id, (now, motion));
                self.slot(index, (span.width, dx), cx)
            })
            .collect();
        div()
            .id("tab-scroller")
            .flex()
            .flex_1()
            .min_w_0()
            .items_center()
            .py(px(SCROLLER_PADDING))
            .my(px(-SCROLLER_PADDING))
            .overflow_x_scroll()
            .track_scroll(&self.scroll)
            .children(slots)
    }

    fn shift(
        &self,
        index: usize,
        tab: TabId,
        (now, motion): (Instant, Motion),
    ) -> f32 {
        if let Some(drag) = &self.drag {
            return drag.slide(index, now, motion);
        }
        self.settle
            .as_ref()
            .filter(|settle| settle.tab == tab)
            .map_or(0.0, |settle| settle.offset(now, motion))
    }

    fn slot(
        &self,
        index: usize,
        (width, dx): (f32, f32),
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let Some(tab) = self.tabs.get(index) else {
            return div().into_any_element();
        };
        let look = self.look_of(index);
        let hidden = self
            .drag
            .as_ref()
            .is_some_and(|drag| drag.engine.tab() == Some(tab.id));
        div()
            .id(("slot", index))
            .relative()
            .left(px(dx))
            .flex_none()
            .w(px(width))
            .px(px(SHELL_MARGIN))
            .when(hidden, Styled::invisible)
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(move |this, event, _, cx| this.pressed(index, event, cx)),
            )
            .on_mouse_down(
                MouseButton::Right,
                cx.listener(move |this, event: &MouseDownEvent, window, cx| {
                    this.open_menu((index, event.position), window, cx);
                }),
            )
            .child(TabShell {
                id: ("tab", index).into(),
                members: tab.members.clone(),
                look,
                width: width - 2.0 * SHELL_MARGIN,
                on_close: Some(Self::close_action(tab.id, cx)),
            })
            .into_any_element()
    }

    fn close_action(
        tab: TabId,
        cx: &mut Context<'_, Self>,
    ) -> CloseAction {
        let view = cx.entity();
        let close = vec![Command::CloseTab { tab }];
        Rc::new(move |_, cx| {
            view.update(cx, |_, cx| cx.emit(TabBarEvent::Apply(close.clone())));
        })
    }

    fn look_of(
        &self,
        index: usize,
    ) -> ShellLook {
        let Some(tab) = self.tabs.get(index) else {
            return ShellLook::Idle;
        };
        let merging = matches!(self.incoming.as_ref().map(|i| i.drop), Some(TabDrop::Merge(id)) if id == tab.id);
        if merging {
            return ShellLook::MergeTarget;
        }
        if self.active == Some(tab.id) {
            return ShellLook::Active;
        }
        ShellLook::Idle
    }

    fn toggle(
        side: SidebarState,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = *Theme::of(cx).palette(Surface::Chrome);
        let button = div()
            .id("sidebar-toggle")
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(TOGGLE_SIZE))
            .rounded(ui::radius::SM)
            .cursor(gpui::CursorStyle::PointingHand)
            .hover(move |style| style.bg(palette.sidebar_element_hover()))
            .on_click(cx.listener(|_, _, _, cx| cx.emit(TabBarEvent::ToggleSidebar)))
            .child(SidebarGlyph {
                color: palette.muted_foreground(),
                mirrored: side == SidebarState::ClosedRight,
            });
        ui::Tooltip::new("sidebar-toggle-tip", "Show Sidebar", button)
    }

    fn ghost(&self) -> Option<AnyElement> {
        let drag = self.drag.as_ref()?;
        let tab = self
            .tabs
            .iter()
            .find(|tab| Some(tab.id) == drag.engine.tab())?;
        let x = drag.engine.ghost_x(self.content_x(drag.pointer));
        let left = x
            + f32::from(self.scroll.offset().x)
            + f32::from(self.strip_left() - self.row.origin.x);
        let row_top = (ROW_HEIGHT - TAB_HEIGHT) / 2.0;
        let top = drag.engine.ghost_y(
            f32::from(drag.pointer.y - self.row.origin.y),
            row_top,
            drag.grab_y + row_top,
        );
        let width = self.slot_spans().first().map_or(0.0, |span| span.width) - 2.0 * SHELL_MARGIN;
        let shell = TabShell {
            id: "tab-ghost".into(),
            members: tab.members.clone(),
            look: ShellLook::Lifted,
            width,
            on_close: None,
        };
        Some(
            deferred(
                div()
                    .absolute()
                    .left(px(left + SHELL_MARGIN))
                    .top(px(top))
                    .child(shell),
            )
            .with_priority(layer::OVERLAY)
            .into_any_element(),
        )
    }

    fn drop_line(
        &self,
        now: Instant,
        motion: Motion,
        cx: &App,
    ) -> Option<AnyElement> {
        let incoming = self.incoming.as_ref()?;
        let TabDrop::Reorder(_) = incoming.drop else {
            return None;
        };
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let x = incoming.line_x(now, motion)
            + f32::from(self.scroll.offset().x)
            + f32::from(self.strip_left() - self.row.origin.x);
        Some(
            div()
                .absolute()
                .left(px(x - 1.0))
                .top(px((ROW_HEIGHT - TAB_HEIGHT) / 2.0 + DROP_LINE_INSET))
                .w(px(2.0))
                .h(px(TAB_HEIGHT - 2.0 * DROP_LINE_INSET))
                .rounded_full()
                .bg(palette.sidebar_drop_line())
                .into_any_element(),
        )
    }

    fn scroll_for_drag(&self) {
        let Some(drag) = &self.drag else {
            return;
        };
        let strip = crate::span::Span {
            x: f32::from(self.strip_left()),
            width: self.strip_width(),
        };
        let step = crate::strip_metrics::edge_scroll_step(f32::from(drag.pointer.x), strip);
        if step != 0.0 {
            let mut offset = self.scroll.offset();
            offset.x -= px(step);
            self.scroll.set_offset(offset);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::TabId;

    use crate::gesture::TabDrop;
    use crate::pill::ShellLook;
    use crate::test_support::{open, view};

    #[gpui::test]
    fn the_active_tab_is_raised_and_a_merge_target_is_marked(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 2);
        let (bar, cx) = (harness.bar, harness.cx);
        bar.read_with(cx, |bar, _| {
            assert_eq!(bar.look_of(1), ShellLook::Active, "active");
            assert_eq!(bar.look_of(0), ShellLook::Idle, "idle");
        });
        bar.update(cx, |bar, _| {
            bar.incoming = Some(crate::gesture::IncomingDrop::new(
                TabDrop::Merge(TabId(1)),
                0.0,
            ));
        });
        bar.read_with(cx, |bar, _| {
            assert_eq!(bar.look_of(0), ShellLook::MergeTarget, "merge target");
        });
    }
}
