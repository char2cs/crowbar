use gpui::TestAppContext;

use super::*;

#[gpui::test]
fn registers_the_three_families(cx: &mut TestAppContext) {
    cx.update(|cx| {
        register(cx).expect("embedded fonts parse");
        let names = cx.text_system().all_font_names();
        for family in [UI_FAMILY, HEADING_FAMILY, MONO_FAMILY] {
            assert!(names.iter().any(|name| name == family), "{family} missing");
        }
    });
}
