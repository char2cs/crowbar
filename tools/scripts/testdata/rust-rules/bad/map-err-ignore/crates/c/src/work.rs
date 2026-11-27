/// Works.
pub fn work(text: &str) -> Result<u32, std::num::ParseIntError> {
    text.parse().map_err(|_| text.parse::<u32>().unwrap_err())
}

#[cfg(test)]
mod tests {}
