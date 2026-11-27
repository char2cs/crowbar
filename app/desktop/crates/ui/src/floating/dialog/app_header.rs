use gpui::prelude::FluentBuilder;
use gpui::{AnyElement, Div, ParentElement, Styled, div};

use crate::floating::dialog::header::Header;
use crate::floating::dialog::header_flow::HeaderFlow;
use crate::foundation::Icon;
use crate::foundation::Palette;
use crate::foundation::tokens::spacing;

impl Header {
    pub(crate) fn heading(
        mut self,
        heading: Option<AnyElement>,
    ) -> Self {
        self.heading = heading;
        self
    }

    pub(crate) fn flow(
        mut self,
        flow: HeaderFlow,
    ) -> Self {
        self.flow = flow;
        self
    }

    pub(crate) fn app(
        self,
        frame: Div,
        (title, close): (Option<Div>, AnyElement),
        palette: &Palette,
    ) -> Div {
        let headed = title.is_some() || self.icon.is_some() || self.heading.is_some();
        let icon = self
            .icon
            .map(|icon| Icon::new(icon).color(palette.muted_foreground()));
        let lead = div()
            .flex()
            .items_center()
            .gap(spacing::STEP_2)
            .min_w_0()
            .children(icon)
            .children(title)
            .children(self.heading)
            .when(self.flow == HeaderFlow::Grid, |lead| {
                lead.min_h(self.controls)
            });
        let actions = self
            .actions
            .map(|actions| actions_slot(actions).when(headed, |slot| slot.ml(spacing::STEP_2)));
        let frame = frame.when(self.ruled, |d| {
            d.border_b_1().border_color(palette.border())
        });
        self.flow.lay_out(frame, lead, (actions, close))
    }
}

fn actions_slot(actions: AnyElement) -> Div {
    div()
        .min_w_0()
        .flex()
        .items_center()
        .gap(spacing::STEP_2)
        .child(actions)
}

#[cfg(test)]
mod tests {
    use gpui::{ElementId, InteractiveElement, IntoElement, TestAppContext, px};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn actions_keep_a_gap_after_a_heading(cx: &mut TestAppContext) {
        let cx = open(cx, move |_, _| {
            let lead = div()
                .debug_selector(|| "lead".to_owned())
                .w(px(80.0))
                .h(px(28.0));
            let actions = div()
                .debug_selector(|| "actions".to_owned())
                .w(px(40.0))
                .h(px(28.0));
            Header::new(ElementId::from("h"))
                .heading(Some(lead.into_any_element()))
                .actions(Some(actions.into_any_element()))
        });
        let lead = cx.debug_bounds("lead").unwrap_or_default();
        let actions = cx.debug_bounds("actions").unwrap_or_default();
        assert_eq!(actions.left(), lead.right() + spacing::STEP_2 * 2.0);
    }
}
