{ pkgs ? import <nixpkgs> {} }:

let
  app = pkgs.buildGoModule {
    pname = "digital-asset-signing-pup";
    version = "0.1.0";
    src = ./.;
    vendorHash = null;

    subPackages = [ "cmd/digital-asset-signing-pup" ];
    doCheck = false;

  };

  run = pkgs.writeScriptBin "run.sh" ''
    #!${pkgs.bash}/bin/bash
    export CONFIG_PATH="''${CONFIG_PATH:-/storage/config.json}"
    exec ${app}/bin/digital-asset-signing-pup
  '';
in
{
  digital-asset-signing-pup = app;
  run = run;
}
