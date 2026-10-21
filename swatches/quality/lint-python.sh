if [[ "${mode}" == correct ]]; then
    ruff check --fix --no-unsafe-fixes .
    ruff format .
fi
ruff format --check .
ruff check --no-fix .
basedpyright --pythonpath "${project_python}" --baselinemode=lock
requirements="$(mktemp "${TMPDIR:-/tmp}/tailor-python-audit.XXXXXX")"
trap 'rm -f "${requirements}"' EXIT
uv export --locked --format requirements-txt --no-emit-project --no-hashes --output-file "${requirements}"
"${selected_nix_python}" -m pip_audit --requirement "${requirements}" --no-deps --disable-pip --strict
