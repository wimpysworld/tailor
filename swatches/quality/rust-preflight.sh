for input in Cargo.toml Cargo.lock; do
    if [[ ! -e "${input}" && ! -L "${input}" ]]; then
        echo "Error: Rust requires root ${input}, but it is missing" >&2
        exit 1
    fi
    if [[ -L "${input}" || ! -f "${input}" ]]; then
        echo "Error: Rust requires a regular root ${input}, not a symlink or non-regular file" >&2
        exit 1
    fi
done
parent="${PWD}"
while [[ "${parent}" != / ]]; do
    if [[ -L "${parent}" || ! -d "${parent}" ]]; then
        echo "Error: Rust project parent ${parent} must be a directory, not a symlink" >&2
        exit 1
    fi
    parent="${parent%/*}"
    parent="${parent:-/}"
done
for tool in cargo rustc [[TAILOR_RUST_TOOLS]]; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        echo "Error: Rust requires ${tool} on PATH" >&2
        exit 1
    fi
done
