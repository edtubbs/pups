{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";

  # The D2 packages live in the dogebox-nur-packages repo (pkgs/d2) as a
  # multi-file package set (default.nix, source.nix, libd2.nix, Cargo.lock),
  # so we fetch the whole repo pinned to a commit rather than a single file.
  #
  # NOTE: the D2 source itself (dogecoinfoundation/d2) is private; the
  # package fetches it over SSH (see pkgs/d2/source.nix in the NUR repo for
  # the deploy-key / sandbox requirements). It cannot be built by public CI.
  dogebox-nur-packages = pkgs.fetchFromGitHub {
    owner = "edtubbs";
    repo = "dogebox-nur-packages";
    rev = "254921d526b359dc5650ffdeb2d0fe6f9415173e";
    hash = "sha256-Y2zz/HPUtT7w9ExampHdmyNrrLg+ZMayhHSrfD7KyvM=";
  };

  d2_bin = pkgs.callPackage "${dogebox-nur-packages}/pkgs/d2" {};

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

    # Prefer a conventionally named binary, otherwise fall back to the
    # first binary the d2 package installs.
    D2_BIN=""
    for candidate in d2-node d2 d2d; do
      if [ -x "${d2_bin}/bin/$candidate" ]; then
        D2_BIN="${d2_bin}/bin/$candidate"
        break
      fi
    done
    if [ -z "$D2_BIN" ]; then
      D2_BIN=$(ls ${d2_bin}/bin/* | head -n 1)
    fi

    echo "Starting D2 node: $D2_BIN" >> ${storageDirectory}/debug.log

    # TODO: pass explicit P2P (42069), RPC (42070) and event (42071) port
    # flags once the d2-node CLI is documented.
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
