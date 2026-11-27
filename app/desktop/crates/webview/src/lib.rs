mod alert;
mod backend;
mod chooser;
mod error;
mod host;
mod menu;
mod scheme;
#[cfg(test)]
mod test_support;
mod view;

pub use alert::{AlertButton, AlertRequest, AlertRole};
pub use backend::{
    AlertAsking, BrowsingData, CaretBlink, CaretReport, ChosenButton, ChosenEntry, ChosenFiles,
    ColorScheme, DesktopSettings, FileChoosing, HostConfig, MenuPopping, PageControl, Platform,
    ViewCapture, ViewCutout, ViewFocus, ViewLayout, ViewLifecycle,
};
pub use chooser::{ChoiceKind, FileChoice, FileFilter};
pub use error::{Error, Result};
pub use host::{Desktop, Handle, Host};
pub use menu::{MenuEdge, MenuEntry, MenuRequest};
pub use scheme::{ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse};
pub use view::{
    Bounds, Color, Config, Corners, Id, IpcHandler, Kind, PageEvent, PageHandler, ParentWindow,
    Snapshot, Visibility,
};

#[cfg(feature = "mock")]
pub use backend::Call;
#[cfg(feature = "mock")]
pub use backend::MockPlatform;
