# Managed by Tailor: nix/python.nix
{ pkgs, ... }:
let
  python = pkgs.python3.withPackages (ps: [
    ps.packaging
    (ps.toPythonModule (pkgs.pip-audit.override { python3 = pkgs.python3; }))
  ]);
in
[
  python
  (pkgs.writeShellScriptBin "tailor-python" ''
    exec ${python}/bin/python3 "$@"
  '')
  pkgs.uv
  pkgs.ruff
  pkgs.basedpyright
]
