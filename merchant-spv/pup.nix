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
  util-linux = pkgs.util-linux;
  optee_libdogecoin = pkgs."libdogecoin-optee-host";

  gatewayScript = pkgs.writeScriptBin "gateway.sh" ''
    #!${pkgs.stdenv.shell}
    set -euo pipefail

    STORAGE="${storageDirectory}"
    OPTEE="${optee_libdogecoin}/bin/optee_libdogecoin"
    SUCH="${libdogecoinPackage}/bin/such"
    SENDTX="${libdogecoinPackage}/bin/sendtx"
    SPVNODE="${libdogecoinPackage}/bin/spvnode"
    JQ="${jq}/bin/jq"
    AWK="${awk}/bin/awk"

    usage() {
        cat <<EOF
Usage: gateway.sh <command> [options]

Commands:
  generateAddress [label]           Generate a new Dogecoin address (using OP-TEE secure enclave)
  listAddresses                     List generated addresses (from file)
  signTransaction <hex>             Sign a raw transaction (using OP-TEE secure enclave)
                    -o <acct>       Account index (default: 0)
                    -l <change>     Change level (default: 0)
                    -i <idx>        Address index (default: 0)
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
            
            # Use OP-TEE secure enclave to generate address
            # Generate address using the secure enclave (account 0, change level 0, next available index)
            if [ ! -f "$STORAGE/address_index.txt" ]; then
                echo "0" > "$STORAGE/address_index.txt"
            fi
            
            INDEX=$(cat "$STORAGE/address_index.txt")
            
            # Generate address using OP-TEE libdogecoin
            address_output=$("$OPTEE" -c generate_address -z -o 0 -l 0 -i "$INDEX" 2>&1)
            address=$(echo "$address_output" | "$AWK" '/Address generated:/ {print $3}')
            
            if [ -z "$address" ]; then
                echo "Error: Failed to generate address using OP-TEE secure enclave"
                echo "Output: $address_output" >&2
                exit 1
            fi
            
            # Increment index for next address
            echo $((INDEX + 1)) > "$STORAGE/address_index.txt"
            
            # Store address (keys are stored securely in OP-TEE, not on filesystem)
            echo "$address|$label|$(date -u +%Y-%m-%dT%H:%M:%SZ)|0|0|$INDEX" >> "$STORAGE/addresses.txt"
            
            # Output JSON
            echo "{\"address\":\"$address\",\"label\":\"$label\",\"created_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"account\":0,\"change_level\":0,\"index\":$INDEX}"
            ;;
            
        listAddresses)
            if [ ! -f "$STORAGE/addresses.txt" ]; then
                echo "[]"
                exit 0
            fi
            
            echo "["
            first=true
            while IFS='|' read -r addr label created acct change idx; do
                if [ "$first" = true ]; then
                    first=false
                else
                    echo ","
                fi
                # Handle both old format (3 fields) and new format (6 fields)
                if [ -z "$acct" ]; then
                    echo -n "{\"address\":\"$addr\",\"label\":\"$label\",\"created_at\":\"$created\"}"
                else
                    echo -n "{\"address\":\"$addr\",\"label\":\"$label\",\"created_at\":\"$created\",\"account\":$acct,\"change_level\":$change,\"index\":$idx}"
                fi
            done < "$STORAGE/addresses.txt"
            echo ""
            echo "]"
            ;;
        
        signTransaction)
            # Sign a raw transaction using OP-TEE secure enclave
            RAW= ACCT=0 CHG=0 IDX=0
            
            while [ $# -gt 0 ]; do
                case "$1" in
                    -t) RAW="$2"; shift 2;;
                    -o) ACCT="$2"; shift 2;;
                    -l) CHG="$2"; shift 2;;
                    -i) IDX="$2"; shift 2;;
                    *) echo "Unknown option for signTransaction: $1"; usage;;
                esac
            done
            
            if [ -z "$RAW" ]; then
                echo "Error: No transaction hex provided"
                usage
            fi
            
            # Sign using OP-TEE
            signed_output=$("$OPTEE" -c sign_transaction -t "$RAW" -o "$ACCT" -l "$CHG" -i "$IDX" 2>&1)
            signed=$(echo "$signed_output" | "$AWK" '/Transaction signed:/ {print $3}')
            
            if [ -z "$signed" ]; then
                echo "Error: Failed to sign transaction using OP-TEE secure enclave"
                echo "Output: $signed_output" >&2
                exit 1
            fi
            
            echo "{\"signed_hex\":\"$signed\",\"account\":$ACCT,\"change_level\":$CHG,\"index\":$IDX}"
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
  inherit nodeStartScript gatewayService healthService logstreamService gatewayScript awk jq host util-linux;
}
