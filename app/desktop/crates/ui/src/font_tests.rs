use gpui::TestAppContext;

use super::*;

#[gpui::test]
fn embedded_files_are_accepted_by_the_text_system(cx: &mut TestAppContext) {
    cx.update(|cx| assert!(register(cx).is_ok()));
}

#[test]
fn family_names_match_the_font_name_tables() {
    for (family, bytes) in [
        (UI_FAMILY, FILES[0]),
        (HEADING_FAMILY, FILES[1]),
        (MONO_FAMILY, FILES[2]),
    ] {
        let needle: Vec<u8> = family.encode_utf16().flat_map(u16::to_be_bytes).collect();
        assert!(
            bytes
                .windows(needle.len())
                .any(|window| window == needle.as_slice()),
            "{family} not in its font file"
        );
    }
}
