use crate::backend::desktop::alert_asking::AlertAsking;
use crate::backend::desktop::desktop_settings::DesktopSettings;
use crate::backend::desktop::file_choosing::FileChoosing;
use crate::backend::desktop::menu_popping::MenuPopping;
use crate::backend::view::browsing_data::BrowsingData;
use crate::backend::view::page_control::PageControl;
use crate::backend::view::view_capture::ViewCapture;
use crate::backend::view::view_cutout::ViewCutout;
use crate::backend::view::view_focus::ViewFocus;
use crate::backend::view::view_layout::ViewLayout;
use crate::backend::view::view_lifecycle::ViewLifecycle;

/// The operations a platform backend offers, as the small capabilities they are made of. One
/// implementation per platform; a recording implementation exists for tests of crates built on
/// top (feature `mock`).
///
/// Backends live on the thread that created them (the GPUI main thread), hence no `Send`.
pub trait Platform:
    ViewLifecycle
    + ViewLayout
    + ViewCutout
    + PageControl
    + ViewFocus
    + ViewCapture
    + BrowsingData
    + DesktopSettings
    + FileChoosing
    + MenuPopping
    + AlertAsking
{
}
