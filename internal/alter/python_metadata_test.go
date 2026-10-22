package alter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonImportIsolation(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3.11+ with packaging is required")
	}
	if err := exec.CommandContext(t.Context(), python, "-I", "-c", "import tomllib, packaging").Run(); err != nil {
		t.Skip("Python 3.11+ with packaging is required")
	}
	helper, err := os.ReadFile("../../swatches/quality/python-preflight.py")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := os.ReadFile("../../swatches/quality/python-preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{"root", "PYTHONPATH"} {
		t.Run(location, func(t *testing.T) {
			root, bin, log := qualityFixture(t, false)
			shadow := root
			if location == "PYTHONPATH" {
				shadow = t.TempDir()
			}
			t.Setenv("PYTHONPATH", shadow)
			t.Setenv("UV_PROJECT_ENVIRONMENT", "")
			for _, name := range []string{"tomllib.py", "packaging/__init__.py", "pip_audit/__main__.py", "sitecustomize.py"} {
				writeManagedTestFile(t, shadow, name, []byte("raise RuntimeError('shadow module imported')\n"))
			}
			writeManagedExecutable(t, filepath.Join(bin, "tailor-python"), "#!/bin/sh\nexec '"+python+"' \"$@\"\n")
			writeManagedExecutable(t, filepath.Join(bin, "uv"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CALLS\"\n")
			writeManagedTestFile(t, root, "uv.lock", []byte("version=1\n"))
			// #nosec G204 -- The interpreter and temporary environment path are controlled by this test.
			create := exec.CommandContext(t.Context(), python, "-I", "-m", "venv", "--without-pip", filepath.Join(root, ".venv"))
			if output, err := create.CombinedOutput(); err != nil {
				t.Fatalf("create environment: %v\n%s", err, output)
			}
			script := strings.NewReplacer("[[TAILOR_PYTHON_TOOLS]]", "", "[[TAILOR_PYTHON_OPERATION]]", "test", "[[TAILOR_PYTHON_HELPER]]", string(helper)).Replace(string(shell))
			for _, valid := range []bool{false, true} {
				metadata := "[project]\nname='fixture'\nrequires-python='>=3.11'\n"
				if valid {
					metadata += "[build-system]\nbuild-backend='hatchling.build'\n"
				}
				writeManagedTestFile(t, root, "pyproject.toml", []byte(metadata))
				command := exec.CommandContext(t.Context(), "bash", "-eu", "-c", script)
				command.Dir = root
				command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CALLS="+log)
				output, err := command.CombinedOutput()
				if (err == nil) != valid || strings.Contains(string(output), "shadow module imported") {
					t.Fatalf("valid=%v: %v\n%s", valid, err, output)
				}
				if !valid {
					if !strings.Contains(string(output), "build backend") || len(managedLintLog(t, log)) != 0 {
						t.Fatalf("metadata rejection did not precede uv: %s", output)
					}
				} else if calls := managedLintLog(t, log); len(calls) != 2 || calls[0] != "lock --check" {
					t.Fatalf("unexpected uv calls: %v", calls)
				}
			}
		})
	}
}

func TestPythonMetadata(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3.11+ with packaging is required for metadata tests")
	}
	probe := exec.CommandContext(t.Context(), python, "-I", "-c", "import tomllib, packaging")
	if err := probe.Run(); err != nil {
		t.Skip("Python 3.11+ with packaging is required for metadata tests")
	}
	helper, err := filepath.Abs("../../swatches/quality/python-preflight.py")
	if err != nil {
		t.Fatal(err)
	}
	script := `
import copy
import os
from pathlib import Path
import sys
scope = {'__name__': 'tailor_test'}
exec(compile(Path(sys.argv[1]).read_text(), sys.argv[1], 'exec'), scope)
metadata, Version = scope['metadata'], scope['Version']
base = {'project': {'name': 'fixture', 'requires-python': '>=3.11'}, 'build-system': {'build-backend': 'hatchling.build'}}
metadata(base, Version('3.13.1'), '3.13')
for requirement in ('', '<4', '>=4', '==3.*', 'broken'):
    data = copy.deepcopy(base)
    data['project']['requires-python'] = requirement
    try:
        metadata(data, Version('3.13.1'))
    except ValueError:
        pass
    else:
        raise AssertionError(requirement)
for request in ('pypy', '3.12', '3.13.2', '3.13\n3.12'):
    try:
        metadata(base, Version('3.13.1'), request)
    except ValueError:
        pass
    else:
        raise AssertionError(request)
for uv in ({'workspace': {}}, {'package': False}, {'sources': {'x': {'git': 'https://invalid'}}}, {'sources': {'x': {'path': '../x'}}}):
    data = copy.deepcopy(base)
    data['tool'] = {'uv': uv}
    try:
        metadata(data, Version('3.13.1'))
    except ValueError:
        pass
    else:
        raise AssertionError(uv)
root = Path(sys.argv[2])
(root / 'pyproject.toml').write_text('[project]\nname="fixture"\nrequires-python=">=3.11"\n[build-system]\nbuild-backend="hatchling.build"\n')
(root / 'uv.lock').write_text('version=1\n')
preflight = scope['preflight']
preflight(root, 'setup', str(root / '.venv/bin/python'))
(root / '.venv').mkdir()
(root / '.venv/pyvenv.cfg').write_text('include-system-site-packages = false\n')
try:
    preflight(root, 'test', str(root / '.venv/bin/python'))
except ValueError as error:
    assert str(error) == 'run just setup: .venv/bin/python is missing or not executable', error
else:
    raise AssertionError('missing interpreter accepted')
import subprocess
import venv
venv.EnvBuilder(with_pip=False).create(root / '.venv')
subprocess.run([str(root / '.venv/bin/python'), '-I', '-c',
    'import importlib.util, sys; '
    'assert sys.prefix != sys.base_prefix; '
    'assert importlib.util.find_spec("packaging") is None'], check=True)
preflight(root, 'test', str(root / '.venv/bin/python'))
from packaging.markers import Marker
import json
from unittest.mock import patch
markers = scope['default_environment']()
(root / 'ruff.toml').write_text('')
(root / 'pyrightconfig.json').write_text('{}')
for field in markers:
    project = dict(markers)
    project[field] = 'different'
    if field == 'python_full_version':
        version = Version(markers[field]).release
        project[field] = '.'.join(map(str, (*version[:2], version[2] + 1)))
        condition = Marker('python_full_version == "' + project[field] + '"')
        assert condition.evaluate(project) and not condition.evaluate(markers)
    with patch.object(scope['subprocess'], 'check_output', side_effect=[markers['python_full_version'], json.dumps(project)]):
        try:
            preflight(root, 'lint', str(root / '.venv/bin/python'))
        except ValueError as error:
            assert 'marker environments' in str(error) and field in str(error), error
        else:
            raise AssertionError('marker mismatch accepted: ' + field)
with patch.dict(scope, find_spec=lambda name: True):
    preflight(root, 'lint', str(root / '.venv/bin/python'))
    nested = root / 'src'
    nested.mkdir()
    (root / 'pyrightconfig.json').unlink()
    for name, content in (
        ('pyrightconfig.json', '{"typeCheckingMode": "off"}'),
        ('pyproject.toml', '[tool.basedpyright]\ntypeCheckingMode="off"\n'),
        ('pyproject.toml', '[tool.pyright]\ntypeCheckingMode="off"\n'),
    ):
        path = nested / name
        path.write_text(content)
        try:
            preflight(root, 'lint', str(root / '.venv/bin/python'))
        except ValueError as error:
            assert 'root invocation does not load nested policies' in str(error), error
        else:
            raise AssertionError('nested type policy allowed implicit root defaults')
        path.unlink()
    (root / 'pyrightconfig.json').write_text('{}')
    (root / 'ruff.toml').unlink()
    for name, content in (
        ('ruff.toml', 'line-length=88\n'),
        ('.ruff.toml', 'line-length=88\n'),
        ('pyproject.toml', '[tool.ruff]\nline-length=88\n'),
    ):
        path = nested / name
        path.write_text(content)
        preflight(root, 'lint', str(root / '.venv/bin/python'))
        assert path.read_text() == content
        path.unlink()
    malformed = nested / 'pyproject.toml'
    malformed.write_text('[tool.ruff\n')
    try:
        preflight(root, 'lint', str(root / '.venv/bin/python'))
    except ValueError:
        pass
    else:
        raise AssertionError('malformed policy accepted without Ruff policy')
    (nested / 'ruff.toml').write_text('line-length=88\n')
    later = nested / 'later'
    later.mkdir()
    (later / 'pyproject.toml').write_text('[tool.ruff\n')
    preflight(root, 'lint', str(root / '.venv/bin/python'))
    assert malformed.read_text() == '[tool.ruff\n'
    assert (later / 'pyproject.toml').read_text() == '[tool.ruff\n'
    (nested / 'ruff.toml').unlink()
    malformed.unlink()
    (later / 'pyproject.toml').unlink()
    later.rmdir()
    (nested / 'pyproject.toml').write_text('[tool.black]\nline-length=88\n')
    try:
        preflight(root, 'lint', str(root / '.venv/bin/python'))
    except ValueError as error:
        assert 'adopt an explicit Ruff policy' in str(error), error
    else:
        raise AssertionError('nested Black policy accepted as Ruff')
os.environ['UV_PROJECT_ENVIRONMENT'] = '/elsewhere'
try:
    preflight(root, 'setup', str(root / '.venv/bin/python'))
except ValueError:
    pass
else:
    raise AssertionError('conflicting environment accepted')
`
	command := exec.CommandContext(t.Context(), python, "-I", "-", helper, t.TempDir())
	command.Stdin = strings.NewReader(script)
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "UV_PROJECT_ENVIRONMENT=")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("metadata tests: %v\n%s", err, output)
	}
}
