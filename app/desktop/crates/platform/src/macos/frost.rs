//! The blur behind a transparent window, as Tauri's `window_chrome.rs` set it up.

use objc2::MainThreadMarker;
use objc2_app_kit::{NSAppearance, NSAppearanceCustomization as _};
use objc2_foundation::NSString;
use window_vibrancy::{NSVisualEffectMaterial, NSVisualEffectState, apply_vibrancy};

use crate::error::Error;
use crate::macos::appkit_parent::AppkitParent;

// PLATFORM: macos the tag `window-vibrancy` gives the blur view it adds.
const BLUR_VIEW_TAG: isize = 91_376_254;

/// The tone the blur behind a window is rendered in.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Frost {
    /// Over a light theme.
    Light,
    /// Over a dark theme.
    Dark,
}

impl Frost {
    // PLATFORM: macos an appearance's name is the string of its own symbol.
    const fn appearance(self) -> &'static str {
        match self {
            Self::Light => "NSAppearanceNameAqua",
            Self::Dark => "NSAppearanceNameDarkAqua",
        }
    }
}

/// Blurs what is behind the window showing the view at `address`, in the tone of `frost`.
///
/// The blur view lies under everything the window's content view holds, and only it takes the
/// appearance: pinning the window's would freeze its web views' colour scheme.
///
/// # Errors
///
/// [`Error::Frost`] off the main thread or when no open window shows the view,
/// [`Error::Vibrancy`] when `AppKit` refuses the effect.
pub fn frost_window(
    address: usize,
    frost: Frost,
) -> Result<(), Error> {
    let main_thread = MainThreadMarker::new().ok_or(Error::Frost("not on the main thread"))?;
    let content = AppkitParent::find(address, main_thread)
        .and_then(|view| view.window())
        .and_then(|window| window.contentView())
        .and_then(|content| AppkitParent::find(std::ptr::from_ref(&*content).addr(), main_thread))
        .ok_or(Error::Frost("no open window shows the view"))?;
    if content.view().viewWithTag(BLUR_VIEW_TAG).is_none() {
        let material = NSVisualEffectMaterial::HudWindow;
        // PLATFORM: macos an inactive effect view paints flat, so the state is held active.
        apply_vibrancy(&content, material, Some(NSVisualEffectState::Active), None)
            .map_err(Error::Vibrancy)?;
    }
    let blur = content
        .view()
        .viewWithTag(BLUR_VIEW_TAG)
        .ok_or(Error::Frost("the blur view is missing"))?;
    let name = NSString::from_str(frost.appearance());
    blur.setAppearance(NSAppearance::appearanceNamed(&name).as_deref());
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_tone_names_its_system_appearance() {
        let cases = [(Frost::Light, "Aqua"), (Frost::Dark, "DarkAqua")];
        for (frost, name) in cases {
            assert_eq!(frost.appearance(), format!("NSAppearanceName{name}"));
        }
    }
}
