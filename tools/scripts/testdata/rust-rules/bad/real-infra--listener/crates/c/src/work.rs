/// Works.
pub fn work() {}
#[cfg(test)]
mod tests {
    #[test]
    fn a() {
        let listener = std::net::TcpListener::bind("127.0.0.1:0");
        drop(listener);
    }
}
