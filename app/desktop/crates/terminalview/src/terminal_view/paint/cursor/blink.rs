use std::time::Duration;

pub(crate) const BLINK_INTERVAL: Duration = Duration::from_millis(530);

#[derive(Debug, Clone, Copy)]
pub(crate) struct Blink {
    shown: bool,
}

impl Default for Blink {
    fn default() -> Self {
        Self { shown: true }
    }
}

impl Blink {
    pub(crate) fn tick(&mut self) {
        self.shown = !self.shown;
    }

    pub(crate) fn restart(&mut self) {
        self.shown = true;
    }

    pub(crate) fn is_shown(self) -> bool {
        self.shown
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ticks_alternate_and_typing_restarts_the_phase() {
        let mut blink = Blink::default();
        assert!(blink.is_shown());
        blink.tick();
        assert!(!blink.is_shown());
        blink.restart();
        assert!(blink.is_shown());
    }
}
