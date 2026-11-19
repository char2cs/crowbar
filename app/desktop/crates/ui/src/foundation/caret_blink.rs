//! How the text caret blinks; the shell sets it from the desktop's settings.

use std::time::Duration;

use gpui::{App, Global};

pub(crate) const GTK_DEFAULT_PHASE: Duration = Duration::from_millis(600);

/// Global caret preference; [`CaretBlink::Every`] the GTK default until the shell sets one.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CaretBlink {
    /// The caret stays shown.
    Steady,
    /// The caret shows, then hides, holding each phase this long.
    Every(Duration),
}

impl Global for CaretBlink {}

impl Default for CaretBlink {
    fn default() -> Self {
        Self::Every(GTK_DEFAULT_PHASE)
    }
}

impl CaretBlink {
    /// The installed preference; the GTK default when none was installed.
    pub fn of(cx: &App) -> Self {
        cx.try_global::<Self>().copied().unwrap_or_default()
    }

    /// How long each phase lasts; `None` for a steady caret.
    pub fn phase(self) -> Option<Duration> {
        match self {
            Self::Steady => None,
            Self::Every(phase) => Some(phase),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn the_installed_preference_wins_over_the_gtk_default(cx: &mut TestAppContext) {
        let read = cx.update(|cx| CaretBlink::of(cx));
        assert_eq!(read.phase(), Some(GTK_DEFAULT_PHASE), "nothing installed");
        cx.update(|cx| cx.set_global(CaretBlink::Steady));
        assert_eq!(cx.update(|cx| CaretBlink::of(cx)).phase(), None, "steady");
    }
}
