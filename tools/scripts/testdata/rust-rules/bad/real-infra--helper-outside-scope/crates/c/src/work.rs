/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    fn helper() {
        let _ = std::fs::read("x");
    }

    #[test]
    #[ignore = "real-infra: reads one real file"]
    fn a() {
        helper();
    }
}
