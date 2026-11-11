//! GTK may only ever run on one thread per process, and tests each get their own thread:
//! every GTK test hands its body to this one thread instead.
use std::panic::{self, AssertUnwindSafe};
use std::sync::{mpsc, OnceLock};

type Job = Box<dyn FnOnce() + Send>;

/// Runs `job` on the process's GTK thread (started on first use) and returns its result.
/// A panic in `job` is the calling test's, and leaves the thread to the next test.
pub fn on_gtk_thread<R: Send + 'static>(job: impl FnOnce() -> R + Send + 'static) -> R {
    static JOBS: OnceLock<mpsc::Sender<Job>> = OnceLock::new();
    let jobs = JOBS.get_or_init(|| {
        let (jobs, queue) = mpsc::channel::<Job>();
        std::thread::spawn(move || {
            gtk::init().expect("GTK needs a display: run the tests under xvfb-run");
            queue.into_iter().for_each(|job| job());
        });
        jobs
    });
    let (result, received) = mpsc::sync_channel(1);
    jobs.send(Box::new(move || {
        let outcome = panic::catch_unwind(AssertUnwindSafe(job));
        result.send(outcome).expect("the test waits for its result");
    }))
    .expect("the GTK thread is running");
    match received.recv().expect("the GTK thread answers every job") {
        Ok(value) => value,
        Err(panicked) => panic::resume_unwind(panicked),
    }
}
