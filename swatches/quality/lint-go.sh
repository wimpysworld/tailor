if [[ "${mode}" == correct ]]; then
    go mod tidy
    golangci-lint fmt --config .golangci.yml
fi
go mod tidy -diff
check_vendor_hash() {
    local file='nix/package.nix' line value='' count=0 computed updated=''
    local assignment='^[[:space:]]*vendorHash[[:space:]]*=[[:space:]]*([^;]*);'
    local literal='^"(sha256-[A-Za-z0-9+/]+=*)"$'
    local unsupported='^[[:space:]]*(proxyVendor[[:space:]]*=[[:space:]]*true|modRoot|overrideModAttrs)'
    if [[ ! -f "${file}" ]]; then
        echo "No ${file} found. Skipping the vendorHash check."
        return 0
    fi
    while IFS= read -r line || [[ -n "${line}" ]]; do
        if [[ "${line}" =~ ${unsupported} ]]; then
            echo "${file} sets ${BASH_REMATCH[1]}. Skipping the vendorHash check."
            return 0
        fi
        if [[ "${line}" =~ ${assignment} ]]; then
            value="${BASH_REMATCH[1]}"
            count=$((count + 1))
        fi
    done < "${file}"
    value="${value%"${value##*[![:space:]]}"}"
    if [[ "${count}" -eq 0 ]]; then
        echo "${file} has no vendorHash. Skipping the vendorHash check."
        return 0
    fi
    if [[ "${count}" -eq 1 && "${value}" == null ]]; then
        echo "${file} sets vendorHash = null. Skipping the vendorHash check."
        return 0
    fi
    if [[ "${count}" -ne 1 || ! "${value}" =~ ${literal} ]]; then
        echo "${file} has no single literal vendorHash. Skipping the vendorHash check."
        return 0
    fi
    value="${BASH_REMATCH[1]}"
    if ! command -v nix > /dev/null; then
        echo "nix is unavailable. Skipping the vendorHash check."
        return 0
    fi
    vendor_hash_work="$(mktemp -d "${TMPDIR:-/tmp}/tailor-go-vendor.XXXXXX")"
    trap 'rm -rf "${vendor_hash_work}"' EXIT
    GOWORK=off go mod vendor -o "${vendor_hash_work}/vendor"
    if [[ ! -f "${vendor_hash_work}/vendor/modules.txt" ]]; then
        echo "Error: go mod vendor found no dependencies. Set vendorHash = null in ${file}" >&2
        return 1
    fi
    computed="$(nix --extra-experimental-features nix-command hash path --sri "${vendor_hash_work}/vendor")"
    if [[ ! "${computed}" =~ ^sha256-[A-Za-z0-9+/]+=*$ ]]; then
        echo "Error: nix hash path returned an unexpected value: ${computed}" >&2
        return 1
    fi
    if [[ "${computed}" == "${value}" ]]; then
        echo "${file} vendorHash matches go.mod and go.sum."
        return 0
    fi
    if [[ "${mode}" != correct ]]; then
        echo "Error: ${file} declares vendorHash ${value}, but go.mod and go.sum give ${computed}" >&2
        echo "Run 'just lint correct', or set vendorHash = \"${computed}\"; in ${file}" >&2
        return 1
    fi
    if [[ -L "${file}" ]]; then
        echo "Error: refusing to update symlink ${file}" >&2
        return 1
    fi
    while IFS= read -r line || [[ -n "${line}" ]]; do
        if [[ "${line}" =~ ${assignment} ]]; then
            line="${line/"${value}"/"${computed}"}"
        fi
        updated+="${line}"$'\n'
    done < "${file}"
    printf '%s' "${updated}" > "${file}"
    echo "Updated vendorHash in ${file} from ${value} to ${computed}."
}
check_vendor_hash
golangci-lint fmt --config .golangci.yml --diff
golangci-lint run --config .golangci.yml
govulncheck -show verbose ./...
gocyclo -top 20 -avg -ignore '_test\.go$' .
