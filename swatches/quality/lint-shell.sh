echo "Shell scope: .sh, .bash, and extensionless sh/bash scripts. Use just/project/lint.sh for fragments and embedded shell."
if (( ${#shell_files[@]} == 0 )); then
    echo "No eligible Shell source files found. Shell checks skipped."
else
    if [[ "${mode}" == correct ]]; then
        for source in "${shell_files[@]}"; do source_regular "${source}"; done
        shfmt --apply-ignore -w -- "${shell_files[@]}"
    fi
    shellcheck -- "${shell_files[@]}"
    shfmt --apply-ignore -d -- "${shell_files[@]}"
fi
