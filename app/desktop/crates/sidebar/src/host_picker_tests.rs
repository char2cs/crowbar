use super::*;
use crate::test_support::prelude::*;

use crate::host::Host;
use crate::host_id::HostId;
use crate::host_kind::HostKind;
use crate::host_state::HostState;
use crate::test_support::measure;

fn host(kind: HostKind) -> Host {
    Host {
        id: HostId::from("h"),
        name: "h".into(),
        kind,
        state: HostState::Up,
        endpoint: None,
    }
}

#[gpui::test]
fn the_host_icon_is_sixteen_pixels_tall(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, cx| {
        let palette = ui::Theme::of(cx).palette(ui::Surface::Chrome);
        host_icon(&host(HostKind::Remote), palette)
    });
    assert_eq!(f32::from(bounds.size.height), 16.0);
}
