use tauri::{AppHandle, Runtime};

pub async fn spawn<R: Runtime>(_app: &AppHandle<R>) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    Ok(())
}
