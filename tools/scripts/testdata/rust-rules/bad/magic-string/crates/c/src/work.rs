/// Works.
pub fn work(x: u32) -> &'static str {
    if x == 1 {
        return "unknown-kind";
    }
    if x == 2 {
        return "unknown-kind";
    }
    "unknown-kind"
}

#[cfg(test)]
mod tests {}
