{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";
  
  libdogecoinPackage = pkgs.callPackage (pkgs.fetchurl {
    url = "https://raw.githubusercontent.com/dogeorg/dogebox-nur-packages/main/pkgs/libdogecoin/default.nix";
    sha256 = "sha256-Dmo2s/LDhJD4S9OO9hhyLi+s0Dv4e4b5wz7WkBtE5kE=";
  }) {};

  awk = pkgs.gawk;
  jq = pkgs.jq;
  host = pkgs.host;

  gatewayScript = pkgs.writeScriptBin "gateway.sh" ''
    #!${pkgs.stdenv.shell}
    set -euo pipefail

    STORAGE="${storageDirectory}"
    SUCH="${libdogecoinPackage}/bin/such"
    SENDTX="${libdogecoinPackage}/bin/sendtx"
    SPVNODE="${libdogecoinPackage}/bin/spvnode"
    JQ="${jq}/bin/jq"
    AWK="${awk}/bin/awk"

    usage() {
        cat <<EOF
Usage: gateway.sh <command> [options]

Commands:
  generateAddress [label]           Generate a new Dogecoin address
  listAddresses                     List generated addresses (from file)
  getAddressBalance <address>       Check balance for an address
  signTransaction <hex> <privkey>   Sign a raw transaction
  broadcastTransaction <hex>        Broadcast a signed transaction
  
Options for broadcastTransaction:
  -i <ip1,ip2,...>                  Explicit peers
  -m <maxPeers>                     Random peer count (default: 5)
  --testnet                         Use testnet
  --regtest                         Use regtest
  --debug                           Verbose logging
  -s <timeoutSecs>                  Timeout in seconds (default: 15)
EOF
        exit 1
    }

    [ $# -gt 0 ] || usage
    cmd="$1"; shift

    case "$cmd" in
        generateAddress)
            label="''${1:-payment}"
            
            # Generate new private key
            privkey_output=$("$SUCH" -c generate_private_key 2>&1)
            privkey=$(echo "$privkey_output" | "$AWK" '/privatekey WIF:/ {print $3}')
            
            if [ -z "$privkey" ]; then
                echo "Error: Failed to generate private key"
                exit 1
            fi
            
            # Generate public key and address
            pubkey_output=$("$SUCH" -c generate_public_key -p "$privkey" 2>&1)
            address=$(echo "$pubkey_output" | "$AWK" '/p2pkh address:/ {print $3}')
            
            if [ -z "$address" ]; then
                echo "Error: Failed to generate address"
                exit 1
            fi
            
            # Store address and key (in production, store securely)
            echo "$address|$label|$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$STORAGE/addresses.txt"
            echo "$address|$privkey" >> "$STORAGE/keys.txt"
            
            # Output JSON
            echo "{\"address\":\"$address\",\"label\":\"$label\",\"created_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
            ;;
            
        listAddresses)
            if [ ! -f "$STORAGE/addresses.txt" ]; then
                echo "[]"
                exit 0
            fi
            
            echo "["
            first=true
            while IFS='|' read -r addr label created; do
                if [ "$first" = true ]; then
                    first=false
                else
                    echo ","
                fi
                echo -n "{\"address\":\"$addr\",\"label\":\"$label\",\"created_at\":\"$created\"}"
            done < "$STORAGE/addresses.txt"
            echo ""
            echo "]"
            ;;
            
        broadcastTransaction)
            TX="$1"; shift
            PEERS=5 IPLIST= NET= DEBUG= TIMEOUT=15
            
            while [ $# -gt 0 ]; do
                case "$1" in
                    -i|-ips) IPLIST="$2"; shift 2;;
                    -m|--maxpeers) PEERS="$2"; shift 2;;
                    --testnet) NET="--testnet"; shift 1;;
                    --regtest) NET="--regtest"; shift 1;;
                    -d|--debug) DEBUG="--debug"; shift 1;;
                    -s|--timeout) TIMEOUT="$2"; shift 2;;
                    *) echo "Unknown option: $1"; usage;;
                esac
            done
            
            SENDOPTS="$DEBUG -s $TIMEOUT $NET"
            if [ -n "$IPLIST" ]; then
                SENDOPTS="$SENDOPTS -i $IPLIST"
            else
                SENDOPTS="$SENDOPTS -m $PEERS"
            fi
            
            "$SENDTX" $SENDOPTS "$TX"
            ;;
            
        *)
            echo "Unknown command: $cmd"
            usage
            ;;
    esac
  '';

  nodeStartScript = pkgs.writeScriptBin "start-node.sh" ''
    #!${pkgs.stdenv.shell}
    
    WALLET_FILE=${storageDirectory}/merchant_wallet.db
    HEADERS_FILE=${storageDirectory}/headers.db
    
    # Ensure storage directory exists
    mkdir -p ${storageDirectory}
    
    # Wait until DNS resolves (basic network check)
    ${host}/bin/host -w seed.multidoge.org || true
    
    # Run spvnode in continuous mode with full sync
    exec ${libdogecoinPackage}/bin/spvnode \
      -c \
      -b \
      -p \
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

    nativeBuildInputs = [ libdogecoinPackage gatewayScript ];
    
    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$TMPDIR/go-cache
      go build -ldflags "-X main.gatewayScript=${gatewayScript}/bin/gateway.sh -X main.storagePath=${storageDirectory}" -o gateway-service gateway.go
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
      go build -ldflags "-X main.libdogecoinPath=${libdogecoinPackage} -X main.storagePath=${storageDirectory}" -o health-service health.go
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
      go build -ldflags "-X main.dataPath=${storageDirectory}" -o logstream-service logstream.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp logstream-service $out/bin/
    '';
  };
in
{
  inherit nodeStartScript gatewayService healthService logstreamService gatewayScript awk jq host;
}
