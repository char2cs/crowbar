//! The wall of the monotonic clock: every animation reads the time through here.

use std::time::Instant;

pub(crate) fn now() -> Instant {
    Instant::now()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn time_never_goes_backwards() {
        let first = now();
        assert!(now() >= first);
    }
}
