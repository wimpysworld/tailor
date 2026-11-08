if (( ${#javascript_files[@]} == 0 )); then
    echo "No eligible JavaScript/TypeScript source files found. Use just/project/lint.sh for additional checks."
else
    if [[ "${mode}" == correct ]]; then
        for source in "${javascript_files[@]}"; do source_regular "${source}"; done
        prettier --write -- "${javascript_files[@]}"
    fi
    oxlint --deny-warnings --no-error-on-unmatched-pattern -- "${javascript_files[@]}"
    prettier --check -- "${javascript_files[@]}"
fi
