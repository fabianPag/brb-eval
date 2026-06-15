#!/bin/bash

N=${1}
OUTPUT_FILE=${2}
ALGORITHM=${3}
PAYLOADSIZE=${4}
T=${5:--1}
D=${6:-0}
TWINSSENDER=${7:-false}
NUMSILENT=${8:-0}
NUMMSGDROPS=${9:-0}
EXPECTED_FINAL_STATE=${10:-0}
CONTROLLER_BIN=${11:-"../../bin/controller"}
NODE_BIN=${12:-"../../bin/node"}
SEED=${13}

if [ -z "$N" ] || [ -z "$OUTPUT_FILE" ]; then
  echo "Usage: $0 <n> <output_file> <algorithm> <payload_size> [t] [d] [twins_sender] [num_silent] [num_msg_drops] [expected_state] [controller_path] [node_path] [seed]"
  exit 1
fi

cat <<EOF > $OUTPUT_FILE
general:
  stop_time: 60s
EOF

if [ -n "$SEED" ]; then
  echo "  seed: $SEED" >> $OUTPUT_FILE
fi

cat <<EOF >> $OUTPUT_FILE

network:
  graph:
    type: 1_gbit_switch

hosts:
  controller:
    network_node_id: 0
    processes:
    - path: $CONTROLLER_BIN
      args: "-addr=controller:9000 -n=$N -algorithm=$ALGORITHM -payloadSize=$PAYLOADSIZE -t=$T -d=$D -twinsSender=$TWINSSENDER -numSilent=$NUMSILENT -numMsgDrops=$NUMMSGDROPS -metricsFile=metrics.json"
      start_time: 0s 
      expected_final_state: {exited: $EXPECTED_FINAL_STATE}
EOF

for (( i=0; i<$N; i++ ))
do
  cat <<EOF >> $OUTPUT_FILE
  peer$i:
    network_node_id: 0
    processes:
    - path: $NODE_BIN
      args: "-id=$i -addr=peer$i:8080 -controller=controller:9000 -measureRTT=false"
      start_time: 1s
      expected_final_state: {exited: $EXPECTED_FINAL_STATE}
EOF
done