use gpui::TestAppContext;
use model::TabId;

use crate::shell_look::ShellLook;
use crate::tab_drop::TabDrop;
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
        bar.incoming = Some(crate::incoming_drop::IncomingDrop::new(
            TabDrop::Merge(TabId(1)),
            0.0,
        ));
    });
    bar.read_with(cx, |bar, _| {
        assert_eq!(bar.look_of(0), ShellLook::MergeTarget, "merge target");
    });
}
