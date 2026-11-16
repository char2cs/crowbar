/// Works.
pub fn work(text: &str) -> u32 {
    match text.parse::<u32>() {
        Ok(value) => value,
        Err(_) => 0,
    }
}

#[cfg(test)]
mod tests {}
