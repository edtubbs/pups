{ pkgs ? import <nixpkgs> {} }:

let
  app = pkgs.buildGoModule {
    pname = "digital-asset-signing-pup";
    version = "0.1.0";
    src = ./.;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      cd cmd/digital-asset-signing-pup
      go build -o digital-asset-signing-pup main.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp cmd/digital-asset-signing-pup/digital-asset-signing-pup $out/bin/
    '';
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
