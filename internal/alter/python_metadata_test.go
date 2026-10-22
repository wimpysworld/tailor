package alter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonMetadata(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3.11+ with packaging is required for metadata tests")
	}
	probe := exec.CommandContext(t.Context(), python, "-c", "import tomllib, packaging")
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
try:
    preflight(root, 'test', str(root / '.venv/bin/python'))
except ValueError:
    pass
else:
    raise AssertionError('missing environment accepted')
(root / '.venv/bin').mkdir(parents=True)
(root / '.venv/pyvenv.cfg').write_text('home=fixture\n')
(root / '.venv/bin/python').symlink_to(sys.executable)
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
	command := exec.CommandContext(t.Context(), python, "-", helper, t.TempDir())
	command.Stdin = strings.NewReader(script)
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "UV_PROJECT_ENVIRONMENT=")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("metadata tests: %v\n%s", err, output)
	}
}
