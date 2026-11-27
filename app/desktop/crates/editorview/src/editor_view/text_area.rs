mod preedit_dropping;
mod whitespace_marks;

use gpui::prelude::FluentBuilder as _;
use gpui::{App, Div, InteractiveElement as _, ParentElement as _, Styled as _, div, px};

use super::{EditorView, TEXT};
use crate::Settings;

impl EditorView {
    pub(super) fn text_area(
        &self,
        cx: &App,
    ) -> Div {
        let settings = Settings::of(cx);
        div()
            .debug_selector(|| TEXT.to_owned())
            .relative()
            .flex_1()
            .min_h_0()
            .font_family(ui::Theme::of(cx).mono_family())
            .text_size(px(settings.font_size))
            .child(self.editor(cx))
            .child(whitespace_marks::layer(&self.state, settings.whitespace))
            .when(cfg!(target_os = "linux"), |text| {
                text.child(preedit_dropping::layer(&self.state))
            })
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::super::BODY;
    use super::super::test_support::opened;
    use crate::Access;

    #[gpui::test]
    fn the_first_line_sits_at_the_top_of_the_editor_with_no_frame_as_develop_s_does(
        cx: &mut TestAppContext
    ) {
        let (view, window) = opened(("a\nb", None), Access::Editable, cx);
        window.run_until_parked();
        let first = window.read(|cx| view.read(cx).state.read(cx).range_to_bounds(&(0..1)));
        let body = window.debug_bounds(BODY);
        assert_eq!(first.map(|first| first.top()), body.map(|body| body.top()));
    }
}
