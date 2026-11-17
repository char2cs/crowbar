pub fn scratch_len() -> usize {
    std::fs::read("x").map(|bytes| bytes.len()).unwrap_or(0)
}
