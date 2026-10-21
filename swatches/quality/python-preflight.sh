export UV_PYTHON_DOWNLOADS=never
unset VIRTUAL_ENV
for tool in tailor-python uv [[TAILOR_PYTHON_TOOLS]]; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        echo "Error: Python requires ${tool} on PATH" >&2
        exit 1
    fi
done
selected_nix_python="$(command -v tailor-python)"
project_python="${PWD}/.venv/bin/python"
"${selected_nix_python}" - "${PWD}" "[[TAILOR_PYTHON_OPERATION]]" "${project_python}" <<'TAILOR_PYTHON'
[[TAILOR_PYTHON_HELPER]]
TAILOR_PYTHON
uv lock --check
if [[ "[[TAILOR_PYTHON_OPERATION]]" != setup ]]; then
    uv sync --check --locked --python "${project_python}" --no-python-downloads
fi
