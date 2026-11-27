/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[test(should_panic)]
    fn a() {}
    #[test(should_panic)]
    fn b() {}
    #[test(should_panic)]
    fn c() {}
}
