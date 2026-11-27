/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[test]
    #[ignore = "real-infra: fs"]
    fn a() {
        let _ = std::fs::read("x");
    }
}
