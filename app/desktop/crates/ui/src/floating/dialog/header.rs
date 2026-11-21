//! Header strip of a dialog: icon, title, extra controls and the close button.

use gpui::{
    AnyElement, App, ElementId, FocusHandle, IntoElement, ParentElement, RenderOnce, SharedString,
    Styled, Window, div, px,
};

use crate::floating::dialog::close_button::close_button;
use crate::floating::dialog::dialog_style::DialogStyle;
use crate::floating::dialog::header_flow::HeaderFlow;
use crate::foundation::CloseHandler;
use crate::foundation::IconName;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::spacing;

#[derive(IntoElement)]
pub(crate) struct Header {
    id: ElementId,
    pub(crate) title: Option<SharedString>,
    pub(crate) icon: Option<IconName>,
    pub(crate) actions: Option<AnyElement>,
    pub(crate) heading: Option<AnyElement>,
    pub(crate) flow: HeaderFlow,
    on_close: Option<CloseHandler>,
    pub(crate) close_focus: Option<FocusHandle>,
    pub(crate) ruled: bool,
    style: DialogStyle,
}

impl Header {
    pub(crate) fn new(id: ElementId) -> Self {
        Self {
            id,
            title: None,
            icon: None,
            actions: None,
            heading: None,
            flow: HeaderFlow::default(),
            on_close: None,
            close_focus: None,
            ruled: true,
            style: DialogStyle::default(),
        }
    }

    pub(crate) fn title(
        mut self,
        title: Option<SharedString>,
    ) -> Self {
        self.title = title;
        self
    }

    pub(crate) fn icon(
        mut self,
        icon: Option<IconName>,
    ) -> Self {
        self.icon = icon;
        self
    }

    pub(crate) fn actions(
        mut self,
        actions: Option<AnyElement>,
    ) -> Self {
        self.actions = actions;
        self
    }

    pub(crate) fn on_close(
        mut self,
        handler: Option<CloseHandler>,
    ) -> Self {
        self.on_close = handler;
        self
    }

    pub(crate) fn style(
        mut self,
        style: DialogStyle,
    ) -> Self {
        self.style = style;
        self
    }

    pub(crate) fn unruled(mut self) -> Self {
        self.ruled = false;
        self
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.title.is_none()
            && self.icon.is_none()
            && self.heading.is_none()
            && self.actions.is_none()
    }
}

impl RenderOnce for Header {
    fn render(
        mut self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let style = self.style;
        let (px_x, px_y) = style.header_padding();
        let title = self
            .title
            .take()
            .map(|title| title_text(title, style, &theme));
        let close = close_button(self.id.clone(), self.on_close.take(), style);
        let close = match self.close_focus.take() {
            Some(focus) => close.focus_handle(focus),
            None => close,
        };
        let frame = div().flex().flex_none().px(px_x).py(px_y);
        match style {
            DialogStyle::Plain => corner_close(frame.children(title), close),
            DialogStyle::Alert => frame.pb(px(0.0)).children(title),
            DialogStyle::App => self.app(frame, (title, close.into_any_element()), palette),
        }
    }
}

fn corner_close(
    frame: gpui::Div,
    close: crate::control::Button,
) -> gpui::Div {
    frame.relative().flex_col().gap(spacing::STEP_2).child(
        div()
            .absolute()
            .right(spacing::STEP_2)
            .top(spacing::STEP_2)
            .child(close),
    )
}

fn title_text(
    title: SharedString,
    style: DialogStyle,
    theme: &Theme,
) -> gpui::Div {
    div()
        .font_family(theme.heading_family())
        .text_size(style.title_size())
        .line_height(style.title_line())
        .child(title)
}

opaque_debug!(Header);

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Modifiers, TestAppContext, point, px};

    use super::*;
    use crate::test_support::probe::open;

    #[test]
    fn emptiness_follows_the_parts() {
        assert!(Header::new(ElementId::from("h")).is_empty());
        assert!(
            !Header::new(ElementId::from("h"))
                .title(Some("Title".into()))
                .is_empty()
        );
        assert!(
            !Header::new(ElementId::from("h"))
                .icon(Some(IconName::Gear))
                .is_empty()
        );
    }

    #[gpui::test]
    fn plain_header_is_title_line_plus_24px_padding_and_app_header_is_ruled(
        cx: &mut TestAppContext
    ) {
        const CASES: &[(DialogStyle, f32)] =
            &[(DialogStyle::Plain, 68.0), (DialogStyle::App, 53.0)];
        for &(style, height) in CASES {
            assert_eq!(header_height(cx, style), height, "{style:?}");
        }
    }

    fn header_height(
        cx: &mut TestAppContext,
        style: DialogStyle,
    ) -> f32 {
        let cx = open(cx, move |_, _| {
            Header::new(ElementId::from("h"))
                .title(Some("T".into()))
                .style(style)
        });
        f32::from(cx.debug_bounds("probe").unwrap_or_default().size.height)
    }

    #[gpui::test]
    fn close_button_calls_the_handler(cx: &mut TestAppContext) {
        let closed = Rc::new(Cell::new(0));
        let sink = Rc::clone(&closed);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            let handler: CloseHandler = Rc::new(move |_, _| sink.set(sink.get() + 1));
            Header::new(ElementId::from("h"))
                .title(Some("T".into()))
                .on_close(Some(handler))
        });
        let bounds = cx.debug_bounds("probe").unwrap_or_default();
        let button = point(bounds.right() - px(16.0 + 14.0), bounds.top() + px(14.0));
        cx.simulate_click(button, Modifiers::none());
        assert_eq!(closed.get(), 1);
    }
}
