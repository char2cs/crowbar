use std::sync::OnceLock;

use tokio::sync::{Mutex, MutexGuard};

fn lock() -> &'static Mutex<()> {
    static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
    LOCK.get_or_init(|| Mutex::new(()))
}

pub(crate) async fn fd_tests() -> MutexGuard<'static, ()> {
    lock().lock().await
}
