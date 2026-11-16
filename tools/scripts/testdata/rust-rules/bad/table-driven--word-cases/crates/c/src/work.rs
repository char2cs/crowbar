/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a() {
        let cases = 3;
        assert_eq!(cases, 3);
        work();
    }
    #[test]
    fn b() {}
    #[test]
    fn c() {}
}
