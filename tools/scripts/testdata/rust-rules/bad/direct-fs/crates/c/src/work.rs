/// Works.
pub fn work() -> Vec<u8> {
    std::fs::read("x").unwrap_or_default()
}

#[cfg(test)]
mod tests {}
