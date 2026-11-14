use std::sync::OnceLock;
use tokio::sync::{Mutex, MutexGuard};

/// Serialises every test that opens a file descriptor: a leak is only measurable
/// process-wide (`/dev/fd`), so a parallel test opening one would read as a leak.
/// tokio's mutex because it is held across awaits and does not poison.
fn lock() -> &'static Mutex<()> {
    static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
    LOCK.get_or_init(|| Mutex::new(()))
}

pub async fn fd_tests() -> MutexGuard<'static, ()> {
    lock().lock().await
}

/// For sync tests, which run outside a runtime (`blocking_lock` panics inside one).
pub fn fd_tests_blocking() -> MutexGuard<'static, ()> {
    lock().blocking_lock()
}
