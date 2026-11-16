/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[tokio::test]
    async fn a() {
        sleep(std::time::Duration::from_millis(5)).await;
    }
}
