{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";

  # The K2 package lives in the dogebox-nur-packages repo (pkgs/k2) as a
  # multi-file package set (default.nix + source.nix), so we fetch the whole
  # repo pinned to a commit rather than a single file.
  #
  # NOTE: the K2 source itself (houseofdoge/km2) is private; the package
  # fetches it over SSH via a fixed-output pkgs.fetchgit derivation (see
  # pkgs/k2/source.nix in the NUR repo for the sandbox/deploy-key
  # requirements). It cannot be built by public CI.
  dogebox-nur-packages = pkgs.fetchFromGitHub {
    owner = "edtubbs";
    repo = "dogebox-nur-packages";
    rev = "0822819fe19040e566de9b27574373d49a49a592";
    hash = "sha256-kdoNccTBfSnxdEaxeTCHsYmZ8M4I0lXDbcrERmGvLbc=";
  };

  k2_bin = pkgs.callPackage "${dogebox-nur-packages}/pkgs/k2" {};

  d2d = pkgs.writeScriptBin "run.sh" ''
    #!${pkgs.stdenv.shell}
    # The d2-node daemon is configured via flags > env (D2_ prefix) > TOML
    # file > defaults. We use env vars to select the testnet chain, keep all
    # node data in pup storage, and bind the ports declared in manifest.json
    # (P2P 42069, RPC 42070) instead of the testnet defaults (44556/44555).
    # The write/admin RPC tier bearer token is written by the node to
    # ${storageDirectory}/rpc.token on first start; the public read tier
    # (d2_getInfo, d2_getHealth, ...) needs no auth.
    export D2_NETWORK=testnet
    export D2_DATADIR=${storageDirectory}
    export D2_LISTENP2P=/ip4/0.0.0.0/tcp/42069
    export D2_RPCLISTEN=0.0.0.0:42070
    export D2_RPCPUBLIC=true

    # Prefer the documented d2-node binary name, otherwise fall back to the
    # first binary the package installs.
    D2_BIN=""
    for candidate in d2-node k2 k2d d2 d2d; do
      if [ -x "${k2_bin}/bin/$candidate" ]; then
        D2_BIN="${k2_bin}/bin/$candidate"
        break
      fi
    done
    if [ -z "$D2_BIN" ]; then
      D2_BIN=$(ls ${k2_bin}/bin/* 2>/dev/null | head -n 1)
    fi
    if [ -z "$D2_BIN" ]; then
      echo "ERROR: no D2 binary found in ${k2_bin}/bin" | tee -a ${storageDirectory}/debug.log >&2
      exit 1
    fi

    echo "Starting D2 node: $D2_BIN (network=testnet)" >> ${storageDirectory}/debug.log

    cd ${storageDirectory}
    HOME=${storageDirectory} exec "$D2_BIN" >> ${storageDirectory}/debug.log 2>&1
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
