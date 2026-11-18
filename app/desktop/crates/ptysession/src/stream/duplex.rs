use tokio::io::{AsyncRead, AsyncWrite};

pub(crate) trait Duplex: AsyncRead + AsyncWrite + Unpin + Send {}

impl<T: AsyncRead + AsyncWrite + Unpin + Send> Duplex for T {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_in_memory_pipe_is_a_duplex() {
        let (near, _far) = tokio::io::duplex(8);
        let _link: Box<dyn Duplex> = Box::new(near);
    }
}
