# Managed by Tailor: nix/lib/blender.nix
{ pkgs, ... }:
let
  version = "0.5.6";
  source = pkgs.applyPatches {
    name = "blender-agent-bridge-local-glb-source";
    src = pkgs.fetchFromGitHub {
      owner = "CallMeJones";
      repo = "blender-agent-bridge";
      rev = "8ef8fc8e0c8220dc812427f9ab89a4634b36cd78";
      hash = "sha256-5/33dOD0qv5QnPgtsOXgXzw0MrDO2i0iyaSwLzZQ1oc=";
    };
    patches = [ ../patches/blender-agent-bridge-local-glb.patch ];
    patchFlags = [
      "-p1"
      "--fuzz=0"
    ];
  };
in
{
  bridge = pkgs.python3Packages.buildPythonApplication {
    pname = "blender-agent-bridge";
    inherit version;
    pyproject = true;
    src = source;
    build-system = with pkgs.python3Packages; [
      setuptools
      wheel
    ];
    pythonImportsCheck = [ "claude_blender.mcp_runtime.server" ];
    doInstallCheck = true;
    installCheckPhase = ''
      runHook preInstallCheck
      export BLENDER_BRIDGE_EXECUTABLE="$out/bin/blender-bridge"
      "$BLENDER_BRIDGE_EXECUTABLE" --help
      python -m unittest tests.unit.test_package_metadata tests.unit.test_mcp_stdio tests.unit.test_local_glb tests.unit.test_matched_views tests.unit.test_tool_registry -v
      runHook postInstallCheck
    '';
  };
  extension = pkgs.runCommand "blender-agent-bridge-extension-${version}" {
    nativeBuildInputs = [ pkgs.python3 ];
  } ''
    mkdir -p "$out"
    python ${source}/scripts/build_extension_zip.py --dist-dir "$out"
  '';
}
