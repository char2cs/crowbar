use gpui::{
    AnyElement, AnyView, Context, InteractiveElement, IntoElement, ParentElement, Styled, Window,
    div, px,
};
use layout::{DEFAULT_CHAT_SHARE, PaneGroup, Presentation, Rect};
use protocol::PaneId;
use ui::Motion;

use crate::layout_view::LayoutView;
use crate::pane_cell::PaneCell;
use crate::pane_frame::PaneFrame;

impl LayoutView {
    pub(crate) fn pane_cells(
        &self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Vec<AnyElement> {
        let Some(tab) = &self.tab else {
            return Vec::new();
        };
        tab.leaves(self.local())
            .into_iter()
            .filter_map(|leaf| self.pane_cell(leaf, window, cx))
            .collect()
    }

    fn pane_cell(
        &self,
        (rect, group): (Rect, &PaneGroup),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let pane = group.shown()?.clone();
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        let frame = PaneFrame::new(rect, self.local(), self.chrome);
        let local = Rect {
            x: 0.0,
            y: 0.0,
            ..frame.hole
        };
        let shown = Presentation::of(group, local, self.chrome.side, DEFAULT_CHAT_SHARE);
        let cell = PaneCell {
            handle: self.handle_for(&pane, (now, motion), cx),
            ring_opacity: self.rings.opacity(&pane, now, motion),
            on_enter: Self::enter_action(&pane, cx),
            on_leave: Self::leave_action(&pane, cx),
            on_press: Self::press_action(&pane, cx),
            content: self.parts((group, &shown), window, cx),
            pane,
            slot: rect,
            frame,
        };
        Some(cell.into_any_element())
    }

    fn parts(
        &self,
        leaf: (&PaneGroup, &Presentation),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let (group, shown) = leaf;
        let chat = group.chat().zip(shown.chat).map(|(pane, at)| {
            let content = self.source.content(pane, window, cx);
            placed(at, pane.as_str(), content)
        });
        let editor = shown
            .editor
            .map(|at| self.editor_part((group, at), window, cx));
        let strip = group.tabs().first().and_then(|tab| self.strip_of(tab, cx));
        let bar = shown
            .tab_bar
            .zip(strip)
            .map(|(at, strip)| placed(at, "tab-bar", strip.into_any_element()));
        div()
            .relative()
            .size_full()
            .children(chat)
            .children(editor)
            .children(bar)
            .into_any_element()
    }

    fn editor_part(
        &self,
        (group, at): (&PaneGroup, Rect),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        if let Some(pane) = group.active_tab() {
            let content = self.editor_view(pane, window, cx);
            return placed(at, pane.as_str(), content);
        }
        let chat = group.chat().map_or("", PaneId::as_str);
        let content = group.chat().map_or_else(
            || gpui::Empty.into_any_element(),
            |chat| self.source.empty_editor(chat, window, cx),
        );
        placed(at, &format!("empty-{chat}"), content)
    }

    fn editor_view(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let content = self.source.content(pane, window, cx);
        let chrome = self.chrome_for(pane, window, cx).map(|(view, height)| {
            div()
                .absolute()
                .top_0()
                .left_0()
                .w_full()
                .h(px(height))
                .child(view)
        });
        div()
            .relative()
            .size_full()
            .child(content)
            .children(chrome)
            .into_any_element()
    }

    fn chrome_for(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<(AnyView, f32)> {
        let height = self.source.chrome_height(pane);
        if height <= 0.0 {
            return None;
        }
        self.source
            .chrome(pane, window, cx)
            .map(|view| (view, height))
    }
}

fn placed(
    at: Rect,
    part: &str,
    content: AnyElement,
) -> AnyElement {
    let selector = format!("pane-part-{part}");
    div()
        .debug_selector(move || selector)
        .absolute()
        .left(px(at.x))
        .top(px(at.y))
        .w(px(at.width))
        .h(px(at.height))
        .child(content)
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use crate::test_support::{layout_of, open, open_tab, pane_id};
    use gpui::TestAppContext;
    use layout::{Command, EditorView};

    #[gpui::test]
    fn a_chat_and_its_active_editor_tab_render_side_by_side_under_one_tab_bar(
        cx: &mut TestAppContext
    ) {
        let mut layout = layout_of("a", &[]);
        open_tab(&mut layout, "t1");
        let harness = open(cx, &layout);
        let cx = harness.cx;
        cx.run_until_parked();
        let chat = cx.debug_bounds("pane-part-a").expect("chat drawn");
        let editor = cx.debug_bounds("pane-part-t1").expect("editor drawn");
        let bar = cx.debug_bounds("pane-part-tab-bar").expect("tab bar drawn");
        assert!(editor.left() > chat.right(), "the editor beside the chat");
        assert_eq!(bar.left(), editor.left(), "the tab bar tops the editor");
    }

    #[gpui::test]
    fn an_open_editor_with_no_tab_draws_the_empty_editor_beside_the_chat(cx: &mut TestAppContext) {
        let mut layout = layout_of("a", &[]);
        let open_editor = Command::SetEditorView {
            pane: pane_id("a"),
            view: EditorView::Open,
        };
        layout.apply(open_editor).expect("open the editor");
        let harness = open(cx, &layout);
        let cx = harness.cx;
        cx.run_until_parked();
        let chat = cx.debug_bounds("pane-part-a").expect("chat drawn");
        let empty = cx
            .debug_bounds("pane-part-empty-a")
            .expect("empty editor drawn");
        assert!(
            empty.left() > chat.right(),
            "the empty editor beside the chat"
        );
    }
}
