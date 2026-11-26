use std::rc::Rc;
use std::sync::Arc;

use crate::backend::MockPlatform;
use crate::backend::ViewLifecycle;
use crate::host::Handle;
use crate::view::Bounds;
use crate::view::Config;
use crate::view::Id;
use crate::view::Kind;
use crate::view::ParentWindow;
use crate::view::Visibility;

pub(crate) fn config(kind: Kind) -> Config {
    let bounds = Bounds {
        x: 1.0,
        y: 2.0,
        width: 3.0,
        height: 4.0,
    };
    Config {
        url: "crowbar://local/".into(),
        bounds,
        kind,
        visibility: Visibility::Hidden,
        background: None,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
        on_page: None,
    }
}

pub(crate) fn handle() -> (Rc<MockPlatform>, Handle) {
    let mock = Rc::new(MockPlatform::default());
    mock.create(Id(0), ParentWindow::X11(1), config(Kind::Core))
        .unwrap();
    (Rc::clone(&mock), Handle::new(mock, Id(0)))
}
