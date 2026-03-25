{ pkgs ? import <nixpkgs> {} }:

let
  exchangePup = pkgs.buildGoModule {
    pname = "exchange-pup";
    version = "0.0.1";
    src = ./.;
    vendorHash = null;

    buildPhase = ''
      export GOCACHE=$(pwd)/.gocache
      go build -o exchange-pup ./cmd/exchange-pup
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp exchange-pup $out/bin/
    '';
  };
in
{
  inherit exchangePup;
}
