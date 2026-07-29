{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";

  # The D2 node package lives in the dogebox-nur-packages repo (pkgs/d2) as
  # a multi-file package set (default.nix + libd2.nix + source.nix), so we
  # fetch the whole repo pinned to a commit rather than a single file. The
  # `d2` attribute builds the d2-node Go daemon (installs bin/d2-node),
  # linked against the libd2 Rust library.
  #
  # NOTE: the d2 source itself (dogecoinfoundation/d2) is private; the
  # package fetches it over SSH via a fixed-output pkgs.fetchgit derivation
  # (see pkgs/d2/source.nix in the NUR repo for the sandbox/deploy-key
  # requirements). It cannot be built by public CI.
  dogebox-nur-packages = pkgs.fetchFromGitHub {
    owner = "edtubbs";
    repo = "dogebox-nur-packages";
    rev = "b6aa812cbbfcf91ed64cf2589d6f220f9894838b";
    hash = "sha256-2uo7PJl+MqhybpEBlTZbsvOvaem4Ye6ezVkyuHTVBhk=";
  };

  # Skip the Go test suite during the pup build: the validator's
  # multi-node libp2p integration test (TestMultiNodeFinalizesOverRealLibp2p)
  # needs real networking between nodes, which the Nix build sandbox does
  # not provide, so it times out at height 0 and fails the install.
  d2_bin = (pkgs.callPackage "${dogebox-nur-packages}/pkgs/d2" {}).overrideAttrs (_: {
    doCheck = false;
  });

  d2d = pkgs.writeScriptBin "run.sh" ''
    #!${pkgs.stdenv.shell}
    # The d2-node daemon is configured via flags > env (D2_ prefix) > TOML
    # file > defaults. We keep all node data in pup storage and bind the
    # ports declared in manifest.json (P2P 42069, RPC 42070) instead of the
    # network defaults. The write/admin RPC tier bearer token is written by
    # the node to ${storageDirectory}/rpc.token on first start; the public
    # read tier (d2_getInfo, d2_getHealth, ...) needs no auth.
    export D2_DATADIR=${storageDirectory}
    export D2_LISTENP2P=/ip4/0.0.0.0/tcp/42069
    export D2_RPCLISTEN=0.0.0.0:42070
    export D2_RPCPUBLIC=true

    CURL=${pkgs.curl}/bin/curl
    JQ=${pkgs.jq}/bin/jq
    SHA256SUM=${pkgs.coreutils}/bin/sha256sum

    LOG=${storageDirectory}/debug.log
    SNAPSHOT_FILE=${storageDirectory}/utxo.dat
    SNAPSHOT_META=${storageDirectory}/utxo.dat.meta.json

    # --- D1 chainstate handoff -------------------------------------------
    # The core pup's snapshot service (core-snapshot interface) publishes a
    # raw dumptxoutset v1 file plus metadata (base blockhash, height, coins
    # count, file sha256). If a new snapshot epoch (different base_hash) is
    # available, download and verify it, wipe the previous chain data
    # (preserving rpc.token and logs), and rebuild genesis from the dump.
    #
    # Chain-correctness verification happens on the core side before the
    # dump is published: d2 itself only checks magic + version + structural
    # decoding. The whole file is read into memory at genesis build and the
    # parse/sort/Merkle work needs several times the file size in RAM; the
    # import is single-shot (a failed import aborts node start — replace
    # the file and restart).
    SNAP_HOST=$DBX_IFACE_CORE_SNAPSHOT_HOST
    SNAP_PORT=$DBX_IFACE_CORE_SNAPSHOT_PORT
    if [ -n "$SNAP_HOST" ] && [ -n "$SNAP_PORT" ]; then
      SNAP_URL="http://$SNAP_HOST:$SNAP_PORT/snapshot"
      META_NEW=$($CURL -fsS --max-time 30 "$SNAP_URL/metadata.json" 2>>$LOG || true)
      # Startup race: on a simultaneous boot (box restart, fresh install) the
      # core pup's snapshot HTTP server may not be listening yet. If there is
      # no local snapshot to fall back on, a one-shot check would strand the
      # node on testnet forever — so keep polling until metadata appears.
      # With a local snapshot present the node can start immediately and the
      # next restart will pick up any new epoch.
      if [ -z "$META_NEW" ] && [ ! -f "$SNAPSHOT_FILE" ]; then
        echo "core-snapshot not reachable and no local snapshot; waiting for snapshot metadata.." >> $LOG
        ATTEMPT=0
        while [ -z "$META_NEW" ]; do
          sleep 15
          ATTEMPT=$((ATTEMPT+1))
          if [ $((ATTEMPT % 20)) -eq 0 ]; then
            echo "Still waiting for core-snapshot metadata (attempt $ATTEMPT).." >> $LOG
          fi
          META_NEW=$($CURL -fsS --max-time 30 "$SNAP_URL/metadata.json" 2>/dev/null || true)
        done
        echo "core-snapshot is now reachable" >> $LOG
      fi
      if [ -n "$META_NEW" ]; then
        NEW_BASE=$(echo "$META_NEW" | $JQ -r .base_hash)
        NEW_SHA=$(echo "$META_NEW" | $JQ -r .file_sha256)
        CUR_BASE=""
        # Only trust the recorded epoch if the snapshot file itself is still
        # present — a stale meta file without utxo.dat must not suppress the
        # (re-)download.
        if [ -f "$SNAPSHOT_META" ] && [ -f "$SNAPSHOT_FILE" ]; then
          CUR_BASE=$($JQ -r .base_hash "$SNAPSHOT_META" 2>/dev/null || true)
        fi
        if [ -n "$NEW_BASE" ] && [ "$NEW_BASE" != "null" ] && [ "$NEW_BASE" != "$CUR_BASE" ]; then
          echo "New D1 snapshot epoch (base $NEW_BASE), downloading.." >> $LOG
          # Resume partial downloads (-C -): the container can be restarted
          # by systemd mid-download, and restarting from byte 0 every time
          # means a large snapshot never finishes. A marker records which
          # epoch the partial belongs to: resuming into a partial from a
          # different epoch would corrupt-concatenate two snapshots.
          if [ -f "$SNAPSHOT_FILE.download" ] && [ "$(cat "$SNAPSHOT_FILE.download.base" 2>/dev/null)" != "$NEW_BASE" ]; then
            echo "Discarding partial download from a previous epoch" >> $LOG
            rm -f "$SNAPSHOT_FILE.download"
          fi
          echo "$NEW_BASE" > "$SNAPSHOT_FILE.download.base"
          if $CURL -fsS -C - -o "$SNAPSHOT_FILE.download" "$SNAP_URL/utxo.dat" 2>>$LOG; then
            GOT_SHA=$($SHA256SUM "$SNAPSHOT_FILE.download" | cut -d' ' -f1)
            if [ "$GOT_SHA" = "$NEW_SHA" ]; then
              # Epoch rollover: reset the chain state so genesis is rebuilt
              # from the new snapshot. Keep credentials, logs and the
              # snapshot artifacts themselves.
              echo "Snapshot verified (sha256 $GOT_SHA); resetting chain data for new epoch" >> $LOG
              for entry in ${storageDirectory}/* ${storageDirectory}/.[!.]*; do
                [ -e "$entry" ] || continue
                case "$(basename "$entry")" in
                  rpc.token|debug.log|utxo.dat|utxo.dat.download|utxo.dat.download.base|utxo.dat.meta.json) ;;
                  *) rm -rf "$entry" ;;
                esac
              done
              mv "$SNAPSHOT_FILE.download" "$SNAPSHOT_FILE"
              echo "$META_NEW" > "$SNAPSHOT_META"
            else
              echo "Snapshot sha256 mismatch: got $GOT_SHA want $NEW_SHA; keeping current chain" >> $LOG
              rm -f "$SNAPSHOT_FILE.download"
              # Without a local snapshot there is no chain to keep — exit and
              # let systemd restart the script to retry the download rather
              # than starting a stranded testnet node.
              if [ ! -f "$SNAPSHOT_FILE" ]; then
                sleep 30
                exit 1
              fi
            fi
          else
            echo "Snapshot download failed; keeping current chain" >> $LOG
            # Same as above: retry via restart (the partial download resumes)
            # instead of falling back to testnet when no snapshot exists yet.
            if [ ! -f "$SNAPSHOT_FILE" ]; then
              sleep 30
              exit 1
            fi
          fi
        fi
      else
        echo "No snapshot metadata available from core-snapshot yet" >> $LOG
      fi
    fi

    D2_BIN=${d2_bin}/bin/d2-node

    cd ${storageDirectory}
    # Run the node without exec: if it exits (e.g. a bad snapshot import or
    # config error), sleep before returning so systemd's restart rate-limit
    # (StartLimitBurst) isn't tripped by an instant crash loop that would
    # leave d2d.service permanently failed.
    STATUS=0
    if [ -f "$SNAPSHOT_FILE" ]; then
      # Genesis-time import: the --d1-snapshot flag is only wired for the
      # regtest devnet genesis path; the raw dumptxoutset file is handed to
      # the node as-is (no conversion step). regtest_devnet requires the
      # network to be regtest — force it via the --network flag (highest
      # config precedence); the D2_NETWORK env var alone was not honored.
      echo "Starting D2 node: $D2_BIN (regtest devnet, d1 snapshot $SNAPSHOT_FILE)" >> $LOG
      HOME=${storageDirectory} "$D2_BIN" --network regtest --regtest-devnet --d1-snapshot "$SNAPSHOT_FILE" >> $LOG 2>&1 || STATUS=$?
    else
      echo "Starting D2 node: $D2_BIN (network=testnet, no d1 snapshot)" >> $LOG
      HOME=${storageDirectory} "$D2_BIN" --network testnet >> $LOG 2>&1 || STATUS=$?
    fi
    if [ "$STATUS" -ne 0 ]; then
      echo "D2 node exited with status $STATUS; backing off 30s before restart" >> $LOG
      sleep 30
    fi
    exit $STATUS
  '';

  monitor = pkgs.buildGoModule {
    pname = "monitor";
    version = "0.0.2";
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
