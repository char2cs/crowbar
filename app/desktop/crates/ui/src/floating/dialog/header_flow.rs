//! How an app dialog's header lays out its heading, actions and close button.

use gpui::{AnyElement, Div, InteractiveElement, ParentElement, Styled, div};

use crate::foundation::tokens::spacing;

/// The arrangement of an app dialog's header.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum HeaderFlow {
    /// One line: heading, actions, then the close button at the end.
    #[default]
    Row,
    /// Two columns: the heading over the close button, the actions beside the heading.
    Grid,
}

impl HeaderFlow {
    pub(crate) fn lay_out(
        self,
        frame: Div,
        lead: Div,
        (actions, close): (Option<Div>, AnyElement),
    ) -> Div {
        let close = div()
            .debug_selector(|| "dialog-close".to_owned())
            .flex_none()
            .ml_auto()
            .child(close);
        match self {
            Self::Row => frame
                .items_center()
                .gap(spacing::STEP_2)
                .child(lead)
                .children(actions.map(Styled::flex_1))
                .child(close),
            Self::Grid => frame
                .items_start()
                .gap(spacing::STEP_2)
                .child(column(lead, close))
                .children(actions.map(Styled::flex_none)),
        }
    }
}

fn column(
    lead: Div,
    close: Div,
) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .flex()
        .flex_col()
        .gap(spacing::STEP_2)
        .child(lead)
        .child(div().flex().child(close))
}

#[cfg(test)]
mod tests {
    use gpui::{ElementId, IntoElement, TestAppContext, px};

    use super::*;
    use crate::floating::dialog::header::Header;
    use crate::test_support::probe::open;

    fn bounds(
        cx: &mut TestAppContext,
        flow: HeaderFlow,
    ) -> (gpui::Bounds<gpui::Pixels>, gpui::Bounds<gpui::Pixels>) {
        let cx = open(cx, move |_, _| {
            Header::new(ElementId::from("h"))
                .heading(Some(
                    div()
                        .debug_selector(|| "lead".to_owned())
                        .w(px(80.0))
                        .h(px(28.0))
                        .into_any_element(),
                ))
                .actions(Some(div().w(px(176.0)).h(px(28.0)).into_any_element()))
                .flow(flow)
        });
        let lead = cx.debug_bounds("lead").unwrap_or_default();
        let close = cx.debug_bounds("dialog-close").unwrap_or_default();
        (lead, close)
    }

    #[gpui::test]
    fn the_flow_puts_the_close_beside_or_under_the_heading(cx: &mut TestAppContext) {
        let (lead, close) = bounds(cx, HeaderFlow::Row);
        assert_eq!(close.top(), lead.top(), "row");
        let (lead, close) = bounds(cx, HeaderFlow::Grid);
        assert_eq!(close.top(), lead.bottom() + spacing::STEP_2, "grid");
    }
}
