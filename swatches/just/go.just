# Managed by Tailor: just/go.just

# Build all Go packages and write executables to bin
build-go:
    #!/usr/bin/env bash
    set -euo pipefail

    list_output="$(go list -f '{{ "{{" }}if eq .Name "main"{{ "}}" }}{{ "{{" }}.ImportPath{{ "}}" }}{{ "{{" }}end{{ "}}" }}' ./...)"
    main_packages=()
    while IFS= read -r package; do
        if [[ -n "${package}" ]]; then
            main_packages+=("${package}")
        fi
    done <<< "${list_output}"

    goexe="$(go env GOEXE)"
    outputs=()
    owners=()

    check_bin_directory() {
        if [[ -L bin ]]; then
            echo "Error: refusing to use symlink bin" >&2
            exit 1
        fi
        if [[ -e bin && ! -d bin ]]; then
            echo "Error: bin exists and is not a directory" >&2
            exit 1
        fi
    }

    check_output_destination() {
        destination="bin/$1"
        if [[ -L "${destination}" ]]; then
            echo "Error: refusing to replace symlink ${destination}" >&2
            exit 1
        fi
        if [[ -e "${destination}" && ! -f "${destination}" ]]; then
            echo "Error: refusing to replace non-regular ${destination}" >&2
            exit 1
        fi
    }

    check_bin_directory
    for package in "${main_packages[@]}"; do
        output="${package##*/}"
        if [[ "${output}" =~ ^v([2-9]|[1-9][0-9]+)$ ]]; then
            parent="${package%/*}"
            output="${parent##*/}"
        fi
        output="${output}${goexe}"
        for index in "${!outputs[@]}"; do
            if [[ "${outputs[${index}]}" == "${output}" ]]; then
                echo "Error: Go executable packages ${owners[${index}]} and ${package} both produce bin/${output}" >&2
                exit 1
            fi
        done
        check_output_destination "${output}"
        owners+=("${package}")
        outputs+=("${output}")
    done

    if (( ${#main_packages[@]} == 0 )); then
        go build ./...
        echo "Go packages built successfully. No executable packages were found."
        exit 0
    fi

    work="$(mktemp -d "${TMPDIR:-/tmp}/tailor-go-build.XXXXXX")"
    trap 'rm -rf "${work}"' EXIT
    mkdir "${work}/validate" "${work}/executables"
    go build -o "${work}/validate/" ./...
    go build -o "${work}/executables/" "${main_packages[@]}"

    check_bin_directory
    mkdir -p bin
    check_bin_directory
    for output in "${outputs[@]}"; do
        check_output_destination "${output}"
    done
    for output in "${outputs[@]}"; do
        check_bin_directory
        check_output_destination "${output}"
        mv "${work}/executables/${output}" "bin/${output}"
    done

# Run Go linters
lint-go:
    @golangci-lint run
    @govulncheck ./...

# Run Go tests
test-go:
    @go test ./...
