//! A text or list field.

use gpui::prelude::FluentBuilder as _;
use gpui::{IntoElement, ParentElement, Styled, div, px};
use settings::{Control, Row};

use crate::frame::Frame;
use crate::frame::RowLayout;

pub(crate) fn field_control(
    frame: &Frame<'_>,
    row: &Row,
) -> impl IntoElement {
    let width = match &row.control() {
        Control::Text { width, .. } | Control::TextList { width, .. } => width.pixels(),
        _ => 0.0,
    };
    div()
        .w(px(width))
        .when(frame.rows == RowLayout::Stacked, Styled::w_full)
        .max_w_full()
        .children(frame.fields.get(row.key()).cloned())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};
    use ui::TextInputEvent;

    #[gpui::test]
    fn an_invalid_search_template_is_kept_but_flagged(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "browser");
        let target = row("browserCustomSearchUrl");
        let typed = TextInputEvent::Changed("not a url".to_owned());
        run(&harness, Intent::Edit(target.clone(), typed), cx);
        run(&harness, Intent::Commit(target.clone()), cx);
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .issue(&target)
            .is_some()));
    }
}
