{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";
  spvnode_bin = pkgs.callPackage (pkgs.fetchurl {
    url = "https://raw.githubusercontent.com/Dogebox-WG/dogebox-nur-packages/90bf9f77add1e033f2749fef21294dd16d9c2cc3/pkgs/libdogecoin/default.nix";
    sha256 = "sha256-l7HHiDlBCpGZy5/ysioCfTJQ21ft+TIP2CATBhgF7nY=";
  }) {
  };

  awk = pkgs.gawk;
  host = pkgs.host;

  spvnode = pkgs.writeScriptBin "run.sh" ''
    #!${pkgs.stdenv.shell}

    # Wait until DNS resolves 'seed.multidoge.org'
    ${host}/bin/host -w seed.multidoge.org 1.1.1.1

    # Run spvnode with smpv from a checkpoint and header-only until sync
    ${spvnode_bin}/bin/spvnode \
      -c -p -l -x \
      -w "${storageDirectory}/wallet.db" \
      -f "${storageDirectory}/headers.db" \
      -u "0.0.0.0:8888" \
      scan 2>&1 | tee -a "${storageDirectory}/output.log"
  '';

  monitor = pkgs.buildGoModule {
    pname = "monitor";
    version = "0.0.1";
    src = ./monitor;
    vendorHash = null;

    systemPackages = [ spvnode_bin ];

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -ldflags "-X main.pathToSpvnode=${spvnode_bin}" -o monitor monitor.go
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
  inherit spvnode monitor logger awk host;
}
