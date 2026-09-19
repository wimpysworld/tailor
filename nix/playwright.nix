# Managed by Tailor: nix/playwright.nix
{ pkgs, ... }:
let
  playwright-driver = pkgs.playwright-driver.overrideAttrs (oldAttrs: {
    passthru = oldAttrs.passthru // {
      browsers = oldAttrs.passthru.browsers-chromium;
    };
  });
  playwright-test = pkgs.playwright-test.overrideAttrs (oldAttrs: {
    installPhase =
      builtins.replaceStrings [ "${pkgs.playwright-driver.browsers}" ] [ "${playwright-driver.browsers}" ]
        oldAttrs.installPhase;
  });
in
[
  (pkgs.playwright-mcp.override {
    inherit playwright-driver playwright-test;
  })
]
