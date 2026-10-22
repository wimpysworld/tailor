# Managed by Tailor: nix/blender.nix
{ pkgs, ... }:
let
  bridge = pkgs.python3Packages.buildPythonApplication {
    pname = "blender-agent-bridge";
    version = "0.5.6";
    pyproject = true;
    src = pkgs.fetchFromGitHub {
      owner = "CallMeJones";
      repo = "blender-agent-bridge";
      rev = "8ef8fc8e0c8220dc812427f9ab89a4634b36cd78";
      hash = "sha256-5/33dOD0qv5QnPgtsOXgXzw0MrDO2i0iyaSwLzZQ1oc=";
    };
    build-system = with pkgs.python3Packages; [
      setuptools
      wheel
    ];
    pythonImportsCheck = [ "claude_blender.mcp_runtime.server" ];
  };
in
[ bridge ]
