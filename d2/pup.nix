{ pkgs ? import <nixpkgs> {} }:

let
  storageDirectory = "/storage";

  # The D2 node package lives in the dogebox-nur-packages repo (pkgs/d2) as
  # a multi-file package set (default.nix + libd2.nix + d2-core.nix +
  # source.nix), so we fetch the whole repo pinned to a commit rather than a
  # single file. D2 has migrated from Go to Rust: the default attribute now
  # builds the d2-node Cargo workspace with rustPlatform.buildRustPackage
  # (installs bin/d2-node), compiling the libd2 crates in-tree via Cargo
  # path dependencies rather than linking a prebuilt library.
  #
  # NOTE: the d2 source itself (dogecoinfoundation/d2) is private; the
  # package fetches it over SSH via a fixed-output pkgs.fetchgit derivation
  # (see pkgs/d2/source.nix in the NUR repo for the sandbox/deploy-key
  # requirements). It cannot be built by public CI.
  dogebox-nur-packages = pkgs.fetchFromGitHub {
    owner = "edtubbs";
    repo = "dogebox-nur-packages";
    rev = "0b9cbd08b70eec1ca5a336e9aff58de4327335bd";
    hash = "sha256-+rtvNLDTd4jwZhGzBpsqoKzPjmdIgjOq/I/He1Xn2JY=";
  };

  # The NUR package already sets doCheck = false: the Rust workspace's
  # multi-node integration tests need real networking between nodes, which
  # the Nix build sandbox does not provide.
  d2_bin = pkgs.callPackage "${dogebox-nur-packages}/pkgs/d2" {};

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
    # Debug-level logging: stdout/stderr are appended to
    # ${storageDirectory}/debug.log below (tailed by the logger service), so
    # the node's step-by-step logs are visible: snapshot header, import pass
    # progress with rate/ETA, "chain engine started", "chain genesis ready",
    # "node started", per-RPC "rpc call" lines, "block produced" and the 60s
    # "node status" heartbeat.
    export D2_LOGLEVEL=debug

    # The d1follow directory is the node's ONLY D1 ingress (there is no D1
    # relay JSON-RPC method). Its sweep runs once a second and drains
    # *.tx files before <height>.blk files, unlinking each one once it is
    # applied:
    #   <height>.blk — canonical raw D1 blocks     (d1follower service)
    #   <txid>.tx    — canonical raw D1 mempool txs (d1mempool service)
    # The node wraps each relayed transaction in the D1Relay D2 transaction
    # type, gossips it through the D2 mempool and includes it in D2 blocks.
    # The relay is OFF by default in the node (d1_relay = false) and
    # requires a follow dir, so it must be enabled explicitly with
    # --d1-relay alongside --d1-follow-dir below.
    # The node's Prometheus /metrics listener exposes the §9.9 follower
    # collectors (d2_d1_height, ...) and the D1 relay counters
    # (d2_d1_relay_txs_total, ...) that d1follower and d1mempool scrape for
    # their checkpoint and GUI metrics. Both are wired as command-line flags
    # below (highest config precedence).
    FOLLOW_DIR=${storageDirectory}/d1follow
    METRICS_LISTEN=127.0.0.1:42072

    CURL=${pkgs.curl}/bin/curl
    JQ=${pkgs.jq}/bin/jq
    SHA256SUM=${pkgs.coreutils}/bin/sha256sum
    TR=${pkgs.coreutils}/bin/tr

    LOG=${storageDirectory}/debug.log
    SNAPSHOT_FILE=${storageDirectory}/utxo.dat
    SNAPSHOT_META=${storageDirectory}/utxo.dat.meta.json

    # Bootstrap peers (manifest config field D2_BOOTSTRAP): a comma-separated
    # list of multiaddrs, e.g.
    # /ip4/1.2.3.4/tcp/42069/p2p/12D3KooW... . The node has no DHT or mDNS
    # discovery, so without a bootstrap list it can only ever accept inbound
    # connections and its gossip mesh stays empty ("Mesh low ... Got 0
    # peers"). The node re-dials the list every 30s while it has no
    # connections and logs bad multiaddrs and dial failures, so a peer that
    # comes up later is picked up automatically. An unset/blank field must
    # not reach the node as an empty peer list.
    D2_BOOTSTRAP="$(echo "''${D2_BOOTSTRAP:-}" | $TR -d '[:space:]')"
    if [ -n "$D2_BOOTSTRAP" ]; then
      export D2_BOOTSTRAP
      echo "Bootstrap peers: $D2_BOOTSTRAP" >> $LOG
    else
      unset D2_BOOTSTRAP
    fi

    D2_BIN=${d2_bin}/bin/d2-node
    PKILL=${pkgs.procps}/bin/pkill
    PGREP=${pkgs.procps}/bin/pgrep

    # --- Reap orphaned nodes from a previous run --------------------------
    # The node's redb store (${storageDirectory}/store.db) is protected by a
    # file lock held for as long as a process has it open, so a second
    # instance dies with "Database already open. Cannot acquire lock.".
    # If this wrapper is killed (supervisor restart, memory pressure) the
    # d2-node child is NOT killed with it, so a fresh wrapper would loop
    # forever against the still-running orphan. Terminate any leftover
    # d2-node before starting a new one, and wait for the lock to be
    # released.
    if $PGREP -f "$D2_BIN" > /dev/null 2>&1; then
      echo "Found a running d2-node from a previous wrapper; terminating it" >> $LOG
      $PKILL -TERM -f "$D2_BIN" >> $LOG 2>&1 || true
      WAITED=0
      while $PGREP -f "$D2_BIN" > /dev/null 2>&1; do
        sleep 2
        WAITED=$((WAITED+2))
        if [ "$WAITED" -ge 60 ]; then
          echo "Previous d2-node still alive after ''${WAITED}s; sending SIGKILL" >> $LOG
          $PKILL -KILL -f "$D2_BIN" >> $LOG 2>&1 || true
          sleep 5
          break
        fi
      done
      echo "Previous d2-node stopped after ''${WAITED}s" >> $LOG
    fi

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

    cd ${storageDirectory}
    # Run the node as a background child (not exec) so this wrapper can
    # forward termination signals to it: on SIGTERM/SIGINT the supervisor
    # would otherwise only kill the shell and leave the node running,
    # holding the redb lock. If the node exits on its own (bad snapshot
    # import or config error), sleep before returning so systemd's restart
    # rate-limit (StartLimitBurst) isn't tripped by an instant crash loop
    # that would leave d2d.service permanently failed.
    STATUS=0
    D2_PID=""
    terminate() {
      if [ -n "$D2_PID" ]; then
        echo "Wrapper received a termination signal; stopping d2-node ($D2_PID)" >> $LOG
        kill -TERM "$D2_PID" 2>/dev/null || true
        wait "$D2_PID" 2>/dev/null || true
      fi
      exit 143
    }
    trap terminate TERM INT HUP
    # The d1follow drop directory must exist before the node arms its
    # follower (the d1follower service also creates it, but the node may
    # start first).
    mkdir -p ${storageDirectory}/d1follow
    if [ -f "$SNAPSHOT_FILE" ]; then
      # Genesis-time import: boot the real testnet chain engine bootstrapped
      # from the D1 UTXO snapshot (D2 has only mainnet, testnet and regtest —
      # there is no "devnet"). The raw dumptxoutset file is handed to the
      # node as-is (no conversion step); --network is forced via flag
      # (highest config precedence).
      #
      # Memory budget: the mainnet snapshot import (203M UTXOs) currently
      # peaks >12G of RAM in pass 2 — use a testnet-sized snapshot until
      # libd2's import memory fix lands. A completed genesis import is
      # persisted in the node's redb store and restored on restart as long
      # as the same snapshot file and validator count are present (this
      # script preserves utxo.dat across restarts and only wipes store.db
      # on an epoch change), so the import only runs once per epoch — but
      # that first import must not be interrupted: give d2d.service a
      # generous memory allowance and raise the systemd start timeout (or
      # disable restart-on-startup-timeout) to avoid restart loops that
      # re-run the import from scratch.
      echo "Starting D2 node: $D2_BIN (network=testnet, d1 snapshot $SNAPSHOT_FILE, follow dir $FOLLOW_DIR, d1 relay on)" >> $LOG
      HOME=${storageDirectory} "$D2_BIN" --network testnet --d1-snapshot "$SNAPSHOT_FILE" \
        --d1-follow-dir "$FOLLOW_DIR" --d1-relay --metrics-listen "$METRICS_LISTEN" >> $LOG 2>&1 &
    else
      echo "Starting D2 node: $D2_BIN (network=testnet, no d1 snapshot, follow dir $FOLLOW_DIR, d1 relay on)" >> $LOG
      HOME=${storageDirectory} "$D2_BIN" --network testnet \
        --d1-follow-dir "$FOLLOW_DIR" --d1-relay --metrics-listen "$METRICS_LISTEN" >> $LOG 2>&1 &
    fi
    D2_PID=$!
    wait "$D2_PID"
    STATUS=$?
    # `wait` returns early (>128) if an untrapped signal interrupts it; keep
    # waiting while the child is still alive so STATUS is its real status.
    while [ "$STATUS" -gt 128 ] && kill -0 "$D2_PID" 2>/dev/null; do
      wait "$D2_PID"
      STATUS=$?
    done
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

  # Feeds the node's d1follow module: polls the core pup's RPC (core-rpc
  # dependency) for confirmed D1 blocks and atomically drops their canonical
  # raw bytes as <height>.blk files into /storage/d1follow, where the node
  # ingests and deletes them. Replaces the retired d2-relay pup, whose
  # d2_sendRawTransaction forwarding of D1 hex could never work (that RPC
  # accepts only canonical D2 transaction bytes).
  d1follower = pkgs.buildGoModule {
    pname = "d1follower";
    version = "0.0.2";
    src = ./d1follower;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -o d1follower d1follower.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp d1follower $out/bin/
    '';
  };

  # Feeds the node's D1 transaction relay: polls the core pup's mempool
  # (core-rpc dependency) for unconfirmed D1 transactions and drops their
  # canonical raw bytes as <txid>.tx into the same follow dir d1follower
  # writes confirmed blocks to. The node wraps each one in a D1Relay D2
  # transaction, gossips it through the D2 mempool and includes it in D2
  # blocks; the relay outcome is read back from its Prometheus counters.
  d1mempool = pkgs.buildGoModule {
    pname = "d1mempool";
    version = "0.0.2";
    src = ./d1mempool;
    vendorHash = null;

    buildPhase = ''
      export GO111MODULE=off
      export GOCACHE=$(pwd)/.gocache
      go build -o d1mempool d1mempool.go
    '';

    installPhase = ''
      mkdir -p $out/bin
      cp d1mempool $out/bin/
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
  inherit d2d monitor logger d1follower d1mempool;
}
