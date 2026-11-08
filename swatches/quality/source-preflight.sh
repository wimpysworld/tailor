for tool in [[TAILOR_SOURCE_TOOLS]]; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        echo "Error: source lint requires ${tool} on PATH" >&2
        exit 1
    fi
done
source_regular() {
    local source="$1" parent
    parent="${source%/*}"
    while [[ "${parent}" != . ]]; do
        if [[ -L "${parent}" || ! -d "${parent}" ]]; then
            echo "Error: source parent ${parent} must be a directory, not a symlink" >&2
            exit 1
        fi
        parent="${parent%/*}"
    done
    if [[ -L "${source}" || ! -f "${source}" ]]; then
        echo "Error: source ${source} must be a regular file, not a symlink" >&2
        exit 1
    fi
}
source_generated() {
    local line comment remainder block=false count=0
    local marker='(^|[[:space:]])(@generated|code generated|do not edit)'
    while (( count < 50 )) && { IFS= read -r line || [[ -n "${line}" ]]; }; do
        (( count += 1 ))
        line="${line#"${line%%[![:space:]]*}"}"
        remainder=''
        if "${block}"; then
            comment="${line}"
        else
            case "${line}" in
                '') continue ;;
                '#!'*) continue ;;
                '#'* ) comment="${line#?}" ;;
                '//'*) comment="${line#??}" ;;
                '/*'*) block=true; comment="${line#??}" ;;
                *) break ;;
            esac
        fi
        if "${block}" && [[ "${comment}" == *'*/'* ]]; then
            remainder="${comment#*\*/}"
            comment="${comment%%\*/*}"
            block=false
        fi
        if grep -Eiq -- "${marker}" <<< "${comment}"; then return 0; fi
        if [[ "${remainder}" == *[![:space:]]* ]]; then break; fi
    done < "$1"
    return 1
}
javascript_files=()
shell_files=()
source_paths="$(mktemp)"
trap 'rm -f -- "${source_paths}"' EXIT
git ls-files --cached --others --exclude-standard -z > "${source_paths}"
while IFS= read -r -d '' source; do
    case "/${source}" in
        */.git/*|*/.cache/*|*/.next/*|*/.nuxt/*|*/.svelte-kit/*|*/.output/*|*/node_modules/*|*/vendor/*|*/third_party/*|*/third-party/*|*/generated/*|*/dist/*|*/build/*|*/coverage/*|*/target/*|*/result/*|*/credentials/*|*/secrets/*) continue ;;
        */.env|*/.env.*|*/.prettierrc|*/.prettierrc.*|*/.eslintrc|*/.eslintrc.*|*/.babelrc|*/.babelrc.*|*/.oxlintrc.json|*/.shellcheckrc|*/.editorconfig|*.config.*|*.generated.*|*.gen.*|*.min.*|*-min.*|*_min.*|*.d.ts|*.d.mts|*.d.cts) continue ;;
    esac
    if [[ -d "${source}" && ! -L "${source}" ]]; then
        indexed_source="$(git --literal-pathspecs ls-files --stage -- "${source}")"
        if [[ -z "${indexed_source}" || "${indexed_source}" == '160000 '* ]]; then continue; fi
    fi
    javascript_source=false
    shell_source=false
    case "${source}" in
        *.js|*.jsx|*.mjs|*.cjs|*.ts|*.tsx|*.mts|*.cts) javascript_source=[[TAILOR_JAVASCRIPT_ENABLED]] ;;
        *.sh|*.bash) shell_source=[[TAILOR_SHELL_ENABLED]] ;;
        *)
            if [[TAILOR_SHELL_ENABLED]] && [[ "${source##*/}" != *.* ]]; then
                shell_source=true
            fi ;;
    esac
    if ! "${javascript_source}" && ! "${shell_source}"; then
        continue
    fi
    if git check-ignore --no-index -q -- "${source}"; then
        continue
    else
        status=$?
        if (( status != 1 )); then exit "${status}"; fi
    fi
    if [[ ! -e "${source}" && ! -L "${source}" ]]; then continue; fi
    source_regular "./${source}"
    first_line=''
    IFS= read -r first_line < "${source}" || true
    if "${shell_source}"; then
        shell_shebang='^#![[:blank:]]*(/[^[:blank:]]*/(ba)?sh|/[^[:blank:]]*/env[[:blank:]]+(-S[[:blank:]]+)?(ba)?sh)([[:blank:]]|$)'
        if [[ "${first_line}" == '#!'* || "${source##*/}" != *.* ]]; then
            if [[ ! "${first_line}" =~ ${shell_shebang} ]]; then continue; fi
        fi
    fi
    if source_generated "./${source}"; then continue; fi
    if "${javascript_source}"; then javascript_files+=("./${source}"); fi
    if "${shell_source}"; then shell_files+=("./${source}"); fi
done < "${source_paths}"
rm -f -- "${source_paths}"
trap - EXIT
