{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";

  # TODO: Replace this placeholder with the real D2 node package once a
  # public release artifact (or dogebox-nur-packages derivation) is
  # available. Expected shape (mirrors the core pup):
  #
  #   d2_bin = pkgs.callPackage (pkgs.fetchurl {
  #     url = "https://raw.githubusercontent.com/Dogebox-WG/dogebox-nur-packages/<rev>/pkgs/d2/default.nix";
  #     sha256 = "<sha256>";
  #   }) {};
  #
  # The run.sh script below should then launch the D2 node with:
  #   - P2P port 42069 (d2-network expose)
  #   - RPC bound to $DBX_PUP_IP on port 42070 (d2-rpc expose)
  #   - event notifications on port 42071 (d2-events expose)
  #   - datadir ${storageDirectory}
  #   - RPC credentials from /storage/rpcuser.txt and /storage/rpcpassword.txt
  d2d = pkgs.writeScriptBin "run.sh" ''
    #!${pkgs.stdenv.shell}
    if [ ! -f /storage/rpcuser.txt ] || [ ! -f /storage/rpcpassword.txt ]; then
        RPCUSER=dogebox_d2_pup_temporary_static_username
        RPCPASS=dogebox_d2_pup_temporary_static_password

        echo "$RPCUSER" > /storage/rpcuser.txt
        echo "$RPCPASS" > /storage/rpcpassword.txt
    else
        RPCUSER=$(cat /storage/rpcuser.txt)
        RPCPASS=$(cat /storage/rpcpassword.txt)
    fi

    echo "D2 node binary is not yet available. This pup is scaffolding for the D2 testnet." >> ${storageDirectory}/debug.log
    echo "Waiting for a D2 release artifact to be published..." >> ${storageDirectory}/debug.log

    while true; do
      sleep 300
    done
  '';

  monitor = pkgs.buildGoModule {
    pname = "monitor";
    version = "0.0.1";
    src = ./monitor;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -o monitor monitor.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp monitor $out/bin/
    '';
  };

  logger = pkgs.buildGoModule {
    pname = "logger";
    version = "0.0.1";
    src = ./logger;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -ldflags "-X main.storageDirectory=${storageDirectory}" -o logger logger.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp logger $out/bin/
    '';
  };
in
{
  inherit d2d monitor logger;
}
