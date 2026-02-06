{ pkgs ? import <nixpkgs> {} }:

let
  persistPath = "/storage";
  
  nodePackage = pkgs.callPackage (pkgs.fetchurl {
    url = "https://raw.githubusercontent.com/dogeorg/dogebox-nur-packages/f989575c285e6b7865c8185405a1abba54cc1964/pkgs/dogecoin-core/default.nix";
    sha256 = "sha256-A3QVCK4OSIHIPZJNUShNsjTc+XffPdRBKeAzIvPeOPY=";
  }) {
    disableWallet = false;
    disableGUI = true;
    disableTests = true;
  };

  nodeStartScript = pkgs.writeScriptBin "start-node.sh" ''
    #!${pkgs.stdenv.shell}
    
    AUTH_USER_FILE=${persistPath}/rpcuser.txt
    AUTH_PASS_FILE=${persistPath}/rpcpassword.txt
    
    if [ ! -f "$AUTH_USER_FILE" ] || [ ! -f "$AUTH_PASS_FILE" ]; then
        echo "merchant_gateway_user" > "$AUTH_USER_FILE"
        echo "merchant_gateway_$(${pkgs.openssl}/bin/openssl rand -hex 16)" > "$AUTH_PASS_FILE"
    fi
    
    RPC_USERNAME=$(cat "$AUTH_USER_FILE")
    RPC_PASSWORD=$(cat "$AUTH_PASS_FILE")
    
    exec ${nodePackage}/bin/dogecoind \
      -port=22556 \
      -datadir=${persistPath} \
      -prune=550 \
      -server=1 \
      -rest=0 \
      -rpcuser="$RPC_USERNAME" \
      -rpcpassword="$RPC_PASSWORD" \
      -rpcbind="$DBX_PUP_IP" \
      -rpcport=22555 \
      -rpcallowip=10.69.0.0/16 \
      -wallet=payments \
      -addresstype=legacy \
      -txindex=0 \
      -dbcache=100
  '';

  gatewayService = pkgs.buildGoModule {
    pname = "gateway-service";
    version = "0.0.1";
    src = ./gateway;
    vendorHash = null;

    nativeBuildInputs = [ nodePackage ];
    
    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$TMPDIR/go-cache
      go build -ldflags "-X main.cliPath=${nodePackage}" -o gateway-service gateway.go
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

    nativeBuildInputs = [ nodePackage ];
    
    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$TMPDIR/go-cache
      go build -ldflags "-X main.binaryPath=${nodePackage}" -o health-service health.go
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
