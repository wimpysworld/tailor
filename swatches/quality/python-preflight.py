import json
import os
import re
import subprocess
import sys
import tomllib
from importlib.util import find_spec
from pathlib import Path

import packaging
from packaging.markers import default_environment
from packaging.requirements import Requirement
from packaging.specifiers import SpecifierSet
from packaging.version import Version


def regular(path, optional=False):
    if optional and not path.exists() and not path.is_symlink():
        return False
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"Python requires a regular {path}, not a symlink")
    return True


def metadata(data, version, requested=None):
    project = data.get("project", {})
    requirement = project.get("requires-python", "")
    if not requirement:
        raise ValueError("declare project.requires-python")
    spec = SpecifierSet(requirement)
    floors = [s for s in spec if s.operator in (">=", "~=", "==")]
    if not floors or any("*" in s.version for s in floors):
        raise ValueError("requires-python needs an explicit >=, ~= or == Python floor")
    if version not in spec:
        raise ValueError(f"Python {version} does not satisfy {requirement}")
    if requested:
        if not re.fullmatch(r"3\.[0-9]+(?:\.[0-9]+)?", requested):
            raise ValueError(
                ".python-version must contain one Python 3 minor or patch version"
            )
        parts = Version(requested).release
        if version.release[: len(parts)] != parts:
            raise ValueError("selected Python does not match .python-version")
    if not data.get("build-system", {}).get("build-backend") or not project.get("name"):
        raise ValueError("Python requires one root package with a build backend")
    uv = data.get("tool", {}).get("uv", {})
    if "workspace" in uv or uv.get("package") is False:
        raise ValueError("uv workspaces and non-package projects are not supported")
    for source in uv.get("sources", {}).values():
        sources = source if isinstance(source, list) else [source]
        if any(set(item) - {"index", "marker", "extra", "group"} for item in sources):
            raise ValueError(
                "local, URL, VCS and workspace dependency sources are not supported"
            )
    groups = list(project.get("optional-dependencies", {}).values())
    groups += list(data.get("dependency-groups", {}).values())
    groups += [project.get("dependencies", []), uv.get("dev-dependencies", [])]
    for group in groups:
        for item in group:
            if isinstance(item, str) and Requirement(item).url:
                raise ValueError(
                    "direct URL, local and VCS dependencies are not supported"
                )


def preflight(root, operation, interpreter):
    for directory in (root, *root.parents):
        if directory.is_symlink() or not directory.is_dir():
            raise ValueError(f"unsafe Python project parent {directory}")
    regular(root / "pyproject.toml")
    regular(root / "uv.lock")
    data = tomllib.loads((root / "pyproject.toml").read_text())
    lock = tomllib.loads((root / "uv.lock").read_text())
    for package in lock.get("package", []):
        source = package.get("source", {})
        if source != {"editable": "."} and set(source) != {"registry"}:
            raise ValueError(
                "uv.lock contains an unsupported local, URL, VCS or workspace source"
            )
    requested = None
    if regular(root / ".python-version", optional=True):
        requested = (root / ".python-version").read_text().strip()
    selected = Version(".".join(map(str, sys.version_info[:3])))
    metadata(data, selected, requested)
    for parent in root.parents:
        path = parent / "pyproject.toml"
        if regular(path, optional=True):
            ancestor = tomllib.loads(path.read_text())
            if "workspace" in ancestor.get("tool", {}).get("uv", {}):
                raise ValueError("uv workspace membership is not supported")
    environment = root / ".venv"
    override = os.environ.get("UV_PROJECT_ENVIRONMENT")
    if override and os.path.abspath(override) != str(environment):
        raise ValueError("unset UV_PROJECT_ENVIRONMENT: Tailor requires the root .venv")
    for directory in (environment, environment / "bin"):
        if directory.is_symlink() or (directory.exists() and not directory.is_dir()):
            raise ValueError(f"unsafe Python environment directory {directory}")
    if operation == "setup":
        regular(environment / "pyvenv.cfg", optional=True)
        return
    regular(environment / "pyvenv.cfg")
    if not Path(interpreter).is_file() or not os.access(interpreter, os.X_OK):
        raise ValueError(
            "run just setup: .venv/bin/python is missing or not executable"
        )
    actual = Version(
        subprocess.check_output(
            [
                interpreter,
                "-I",
                "-c",
                "import sys; print('.'.join(map(str, sys.version_info[:3])))",
            ],
            text=True,
        ).strip()
    )
    metadata(data, actual, requested)
    if operation != "lint":
        return
    packaging_path = str(Path(packaging.__path__[0]).parent)
    project_markers = json.loads(
        subprocess.check_output(
            [
                interpreter,
                "-I",
                "-c",
                "import sys, json; "
                f"sys.path.insert(0, {packaging_path!r}); "
                "from packaging.markers import default_environment; "
                "print(json.dumps(default_environment()))",
            ],
            text=True,
        )
    )
    audit_markers = default_environment()
    if project_markers != audit_markers:
        differences = ", ".join(
            key
            for key in sorted(audit_markers)
            if project_markers.get(key) != audit_markers[key]
        )
        raise ValueError(
            f"audit requires matching project and Nix Python marker environments "
            f"({differences}); rebuild .venv with just setup"
        )
    if find_spec("pip_audit") is None:
        raise ValueError("the Nix Python must provide pip-audit")
    policy = False
    ruff_policy = False
    for directory in (root, *root.parents):
        path = directory / "pyrightconfig.json"
        if regular(path, optional=True):
            policy = True
        path = directory / "pyproject.toml"
        if regular(path, optional=True):
            tool = tomllib.loads(path.read_text()).get("tool", {})
            policy = policy or "basedpyright" in tool or "pyright" in tool
            ruff_policy = ruff_policy or "ruff" in tool
        for name in (".ruff.toml", "ruff.toml"):
            ruff_policy = regular(directory / name, optional=True) or ruff_policy
    if not policy:
        raise ValueError(
            "adopt an explicit BasedPyright policy at the project root or an ancestor "
            "before Python lint; the root invocation does not load nested policies; "
            "mypy policy is not converted"
        )
    if not ruff_policy:
        for directory, children, _ in os.walk(root):
            children[:] = [
                name
                for name in children
                if not name.startswith(".")
                and name not in ("node_modules", "vendor")
                and not (Path(directory) / name).is_symlink()
            ]
            directory = Path(directory)
            if directory == root:
                continue
            for name in (".ruff.toml", "ruff.toml", "pyproject.toml"):
                path = directory / name
                if regular(path, optional=True):
                    data = tomllib.loads(path.read_text())
                    ruff_policy = (
                        ruff_policy
                        or name != "pyproject.toml"
                        or "ruff" in data.get("tool", {})
                    )
    if not ruff_policy:
        raise ValueError(
            "adopt an explicit Ruff policy before Python lint; Black policy is not converted"
        )


if __name__ == "__main__":
    try:
        preflight(Path(sys.argv[1]), sys.argv[2], sys.argv[3])
    except (ValueError, OSError, ImportError, subprocess.CalledProcessError) as error:
        sys.exit(f"Error: {error}")
