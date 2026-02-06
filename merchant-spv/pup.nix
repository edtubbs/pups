{ pkgs ? import <nixpkgs> {} }:

let
  persistPath = "/storage";
  
  libdogecoinPackage = pkgs.callPackage (pkgs.fetchurl {
    url = "https://raw.githubusercontent.com/dogeorg/dogebox-nur-packages/main/pkgs/libdogecoin/default.nix";
    sha256 = "sha256-Dmo2s/LDhJD4S9OO9hhyLi+s0Dv4e4b5wz7WkBtE5kE=";
  }) {};

  nodeStartScript = pkgs.writeScriptBin "start-node.sh" ''
    #!${pkgs.stdenv.shell}
    
    WALLET_FILE=${persistPath}/merchant_wallet.db
    HEADERS_FILE=${persistPath}/headers.db
    
    # Ensure storage directory exists
    mkdir -p ${persistPath}
    
    # Run spvnode in continuous mode with full sync
    exec ${libdogecoinPackage}/bin/spvnode \
      -c \
      -b \
      -w "$WALLET_FILE" \
      -h "$HEADERS_FILE" \
      -l \
      scan
  '';

  gatewayService = pkgs.buildGoModule {
    pname = "gateway-service";
    version = "0.0.1";
    src = ./gateway;
    vendorHash = null;

    nativeBuildInputs = [ libdogecoinPackage ];
    
    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$TMPDIR/go-cache
      go build -ldflags "-X main.libdogecoinPath=${libdogecoinPackage} -X main.storagePath=${persistPath}" -o gateway-service gateway.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp gateway-service $out/bin/
    '';
  };

  healthService = pkgs.buildGoModule {
    pname = "health-service";
    version = "0.0.1";
    src = ./health;
    vendorHash = null;

    nativeBuildInputs = [ libdogecoinPackage ];
    
    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$TMPDIR/go-cache
      go build -ldflags "-X main.libdogecoinPath=${libdogecoinPackage} -X main.storagePath=${persistPath}" -o health-service health.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp health-service $out/bin/
    '';
  };

  logstreamService = pkgs.buildGoModule {
    pname = "logstream-service";
    version = "0.0.1";
    src = ./logstream;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$TMPDIR/go-cache
      go build -ldflags "-X main.dataPath=${persistPath}" -o logstream-service logstream.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp logstream-service $out/bin/
    '';
  };
in
{
  inherit nodeStartScript gatewayService healthService logstreamService;
}
