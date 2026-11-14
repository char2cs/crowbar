use gpui::TestAppContext;

use super::*;
use crate::assets::Assets;

#[gpui::test]
fn init_installs_the_globals_and_registers_fonts(cx: &mut TestAppContext) {
    cx.update(|cx| {
        assert!(init(cx).is_ok());
        assert!(cx.try_global::<Theme>().is_some());
        assert!(cx.try_global::<Motion>().is_some());
    });
    let _ = Assets;
}
