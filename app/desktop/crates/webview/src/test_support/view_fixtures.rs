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

pub(crate) fn handle() -> (Rc<MockPlatform>, Handle) {
    let mock = Rc::new(MockPlatform::default());
    let bounds = Bounds {
        x: 0.0,
        y: 0.0,
        width: 1.0,
        height: 1.0,
    };
    let config = Config {
        url: "x".into(),
        bounds,
        kind: Kind::Core,
        visibility: Visibility::Shown,
        background: None,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
        on_page: None,
    };
    mock.create(Id(0), ParentWindow::X11(1), config).unwrap();
    (Rc::clone(&mock), Handle::new(mock, Id(0)))
}
