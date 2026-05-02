#!/bin/bash

NUM_PEERS=${1}
OUTPUT_FILE=${2}
ALGORITHM=${3}
PAYLOADSIZE=${4}
T=${5:--1}
D=${6:-0}
TWINSSENDER=${7:-false}
NUMSILENT=${8:-0}
NUMMSGDROPS=${9:-0}
EXPECTED_FINAL_STATE=${10:-0}

if [ -z "$NUM_PEERS" ] || [ -z "$OUTPUT_FILE" ]; then
  echo "Usage: $0 <num_peers> <output_file> <algorithm> <payload_size> [t] [d] [twins_sender] [num_silent] [num_msg_drops]"
  echo "Algorithms: bracha, afrt, codedmbrb"
  exit 1
fi

cat <<EOF > $OUTPUT_FILE
general:
  stop_time: 60s

network:
  graph:
    type: 1_gbit_switch

hosts:
  controller:
    network_node_id: 0
    processes:
    - path: ../../bin/controller
      args: "-addr=controller:9000 -targetNodes=$NUM_PEERS -algorithm=$ALGORITHM -payloadSize=$PAYLOADSIZE -t=$T -d=$D -twinsSender=$TWINSSENDER -numSilent=$NUMSILENT -numMsgDrops=$NUMMSGDROPS"
      start_time: 0s 
      expected_final_state: {exited: $EXPECTED_FINAL_STATE}
EOF

for (( i=0; i<$NUM_PEERS; i++ ))
do
  cat <<EOF >> $OUTPUT_FILE
  peer$i:
    network_node_id: 0
    processes:
    - path: ../../bin/node
      args: "-id=$i -addr=peer$i:8080 -controller=controller:9000"
      start_time: 1s
      expected_final_state: {exited: $EXPECTED_FINAL_STATE}
EOF
done
