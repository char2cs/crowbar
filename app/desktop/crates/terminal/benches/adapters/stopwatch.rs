use std::time::Instant;

pub(crate) struct Stopwatch {
    started: Instant,
}

impl Stopwatch {
    pub(crate) fn start() -> Self {
        Self {
            started: Instant::now(),
        }
    }

    pub(crate) fn elapsed_seconds(&self) -> f64 {
        self.started.elapsed().as_secs_f64()
    }
}
