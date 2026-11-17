/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[tokio::test(flavor = "current_thread")]
    fn a() {}
    #[tokio::test(flavor = "current_thread")]
    fn b() {}
    #[tokio::test(flavor = "current_thread")]
    fn c() {}
}
