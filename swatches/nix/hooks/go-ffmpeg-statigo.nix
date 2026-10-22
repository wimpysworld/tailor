# Managed by Tailor: nix/hooks/go-ffmpeg-statigo.nix
{ pkgs, ... }:
pkgs.lib.optionalString pkgs.stdenv.isLinux ''
  if tailor_nixos_drivers; then
    tailor_prepend_path LD_LIBRARY_PATH "${pkgs.lib.optionalString pkgs.stdenv.hostPlatform.isx86_64 "${pkgs.vpl-gpu-rt}/lib:${pkgs.intel-media-driver}/lib:"}${pkgs.vulkan-loader}/lib"
    ${pkgs.lib.optionalString pkgs.stdenv.hostPlatform.isx86_64 ''
      if [ "''${ONEVPL_SEARCH_PATH+x}" != x ]; then
        export ONEVPL_SEARCH_PATH="${pkgs.vpl-gpu-rt}/lib"
      fi
    ''}
    if [ "''${LIBVA_DRIVERS_PATH+x}" != x ]; then
      ${pkgs.lib.optionalString pkgs.stdenv.hostPlatform.isx86_64 ''
        export LIBVA_DRIVERS_PATH="${pkgs.intel-media-driver}/lib/dri"
      ''}
      for tailor_va_driver in /run/opengl-driver/lib/dri/${
        if pkgs.stdenv.hostPlatform.isx86_64 then "iHD_drv_video.so" else "*_drv_video.so"
      }; do
        if [ -f "$tailor_va_driver" ] && [ -r "$tailor_va_driver" ]; then
          export LIBVA_DRIVERS_PATH=/run/opengl-driver/lib/dri
          break
        fi
      done
      unset tailor_va_driver
    fi
    if [ "''${VK_DRIVER_FILES+x}" != x ] && [ -d /run/opengl-driver/share/vulkan/icd.d ]; then
      tailor_icds=""
      while IFS= read -r tailor_icd; do
        if [ -r "$tailor_icd" ]; then
          tailor_icds="''${tailor_icds:+$tailor_icds:}$tailor_icd"
        fi
      done < <(find -L /run/opengl-driver/share/vulkan/icd.d -type f -name '*.json' 2>/dev/null | LC_ALL=C sort)
      if [ -n "$tailor_icds" ]; then export VK_DRIVER_FILES="$tailor_icds"; fi
      unset tailor_icds tailor_icd
    fi
  fi
''
