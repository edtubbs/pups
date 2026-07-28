{ pkgs ? import <nixpkgs> {} }:

let
  relay = pkgs.buildGoModule {
    pname = "relay";
    version = "0.0.3";
    src = ./relay;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -o relay relay.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp relay $out/bin/
    '';
  };
in
{
  inherit relay;
}
