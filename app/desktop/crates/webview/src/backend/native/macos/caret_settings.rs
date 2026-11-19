//! The caret blink of `AppKit` text fields.

use std::time::Duration;

use crate::backend::caret_blink::{CaretBlink, CaretReport};
use crate::backend::desktop_settings::DesktopSettings;
use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::error::Result;

// PLATFORM: macos AppKit's phase with NSTextInsertionPointBlinkPeriodOn/Off unset; not read yet.
const APPKIT_PHASE: Duration = Duration::from_millis(567);

fn appkit_blink() -> CaretBlink {
    CaretBlink::Every(APPKIT_PHASE)
}

impl DesktopSettings for AppkitPlatform {
    fn follow_caret(
        &self,
        report: CaretReport,
    ) -> Result<()> {
        report(appkit_blink());
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn appkit_fields_blink_at_the_system_default() {
        assert_eq!(appkit_blink(), CaretBlink::Every(APPKIT_PHASE));
    }
}
