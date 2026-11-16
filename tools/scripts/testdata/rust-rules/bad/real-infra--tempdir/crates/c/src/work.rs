/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[test]
    fn a() {
        let dir = tempfile::tempdir().unwrap();
        drop(dir);
    }
}
