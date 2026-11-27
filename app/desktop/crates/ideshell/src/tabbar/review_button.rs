use ui::IconName;

use crate::tabbar::tab_actions::TabAction;
use crate::toprow::row_button::RowButton;
use crate::toprow::row_ink::RowInk;

pub(crate) fn review_button(
    name: &str,
    on_review: TabAction,
) -> RowButton {
    RowButton {
        id: format!("review-{name}").into(),
        icon: IconName::GitPullRequest,
        tip: "Review this branch".into(),
        ink: RowInk::Muted,
        on_click: on_review,
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use super::*;

    #[test]
    fn the_shortcut_is_develop_s_pull_request_glyph_named_review_this_branch() {
        let button = review_button("t0", Rc::new(|_, _| {}));
        assert_eq!(
            (button.id.as_ref(), button.icon, button.tip.as_ref()),
            ("review-t0", IconName::GitPullRequest, "Review this branch")
        );
    }
}
