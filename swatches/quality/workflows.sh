check_workflows() {
    for workflow in .github/workflows/* .github/workflows/.*; do
        case "${workflow}" in *.yml|*.yaml)
            if [[ -f "${workflow}" ]]; then
                actionlint
                return
            fi ;;
        esac
    done
    echo "No GitHub Actions workflows found. Skipping actionlint."
}
