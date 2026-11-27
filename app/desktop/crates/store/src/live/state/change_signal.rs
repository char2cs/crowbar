use std::sync::Arc;

pub type ChangeSignal = Arc<dyn Fn() + Send + Sync>;
