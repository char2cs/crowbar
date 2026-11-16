/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[test]
    fn a() {
        let _ = std::fs::read("x");
    }
}
