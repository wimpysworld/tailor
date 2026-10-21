if [[ "${mode}" == correct ]]; then
    cargo fmt --all
fi
cargo fmt --all -- --check
cargo clippy --workspace --all-targets --locked -- -D warnings
cargo audit --file Cargo.lock
