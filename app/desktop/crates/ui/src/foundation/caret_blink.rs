use std::time::Duration;

use gpui::{App, Global};

pub(crate) const GTK_DEFAULT_PHASE: Duration = Duration::from_millis(600);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CaretBlink {
    Steady,
    Every(Duration),
}

impl Global for CaretBlink {}

impl Default for CaretBlink {
    fn default() -> Self {
        Self::Every(GTK_DEFAULT_PHASE)
    }
}

impl CaretBlink {
    pub fn of(cx: &App) -> Self {
        cx.try_global::<Self>().copied().unwrap_or_default()
    }

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
