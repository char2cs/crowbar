#[derive(Debug, Clone)]
pub struct ReadError {
    pub message: String,
    pub denied: bool,
}
