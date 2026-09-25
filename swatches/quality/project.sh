project_script='just/project/[[TAILOR_OPERATION]].sh'
for parent in just just/project; do
    if [[ -L "${parent}" || ( -e "${parent}" && ! -d "${parent}" ) ]]; then
        echo "Error: project script parent ${parent} must be a directory, not a symlink" >&2
        exit 1
    fi
done
if [[ -L "${project_script}" || ( -e "${project_script}" && ! -f "${project_script}" ) ]]; then
    echo "Error: project script ${project_script} must be a regular file, not a symlink" >&2
    exit 1
fi
if [[ -f "${project_script}" ]]; then
    bash "${project_script}" [[TAILOR_ARGUMENTS]]
fi
