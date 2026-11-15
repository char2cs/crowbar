/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[test]
    fn a() {
        let _h = std::thread::sleep(std::time::Duration::ZERO);
    }
}
