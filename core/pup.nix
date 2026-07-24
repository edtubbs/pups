{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";
  # Dogecoin Core with the dumptxoutset/loadtxoutset backport (builds
  # edtubbs/dogecoin branch copilot/backport-dumptxoutset-loadtxoutset,
  # pinned by commit inside the package). The backport adds the
  # `dumptxoutset` RPC used by the snapshot service below to export the
  # chainstate for the D2 testnet handoff.
  dogecoind_bin = pkgs.callPackage (pkgs.fetchurl {
    url = "https://raw.githubusercontent.com/edtubbs/dogebox-nur-packages/26a625f3a00a827619359620619b172f86c29f93/pkgs/dogecoin-core-backport/default.nix";
    sha256 = "sha256-+AO/cy976pcDIGvk/aSRxNLI9B0ehnigPUG3qn0KTrg=";
  }) {
    disableWallet = true;
    disableGUI = true;
    disableTests = true;
    enableZMQ = true;
  };

  dogecoind = pkgs.writeScriptBin "run.sh" ''
    #!${pkgs.stdenv.shell}
    if [ ! -f /storage/rpcuser.txt ] || [ ! -f /storage/rpcpassword.txt ]; then
        RPCUSER=dogebox_core_pup_temporary_static_username
        RPCPASS=dogebox_core_pup_temporary_static_password

        echo "$RPCUSER" > /storage/rpcuser.txt
        echo "$RPCPASS" > /storage/rpcpassword.txt
    else
        RPCUSER=$(cat /storage/rpcuser.txt)
        RPCPASS=$(cat /storage/rpcpassword.txt)
    fi
    
    ${dogecoind_bin}/bin/dogecoind \
      -port=22556 \
      -datadir=${storageDirectory} \
      -rpc=1 \
      -rpcuser=$RPCUSER \
      -rpcpassword=$RPCPASS \
      -rpcbind=$DBX_PUP_IP \
      -rpcport=22555 \
      -rpcallowip=0.0.0.0/0 \
      -zmqpubhashblock=tcp://0.0.0.0:28332
  '';

  monitor = pkgs.buildGoModule {
    pname = "monitor";
    version = "0.0.1";
    src = ./monitor;
    vendorHash = null;

    systemPackages = [ dogecoind_bin ];
    
    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -ldflags "-X main.pathToDogecoind=${dogecoind_bin}" -o monitor monitor.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp monitor $out/bin/
    '';
  };

  snapshot = pkgs.buildGoModule {
    pname = "snapshot";
    version = "0.0.1";
    src = ./snapshot;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -ldflags "-X main.storageDirectory=${storageDirectory}" -o snapshot snapshot.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp snapshot $out/bin/
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
  inherit dogecoind monitor snapshot logger;
}
