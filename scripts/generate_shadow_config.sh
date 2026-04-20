#!/bin/bash

NUM_PEERS=${1}
OUTPUT_FILE=${2}
ALGORITHM=${3}
PAYLOADSIZE=${4}

if [ -z "$NUM_PEERS" ] || [ -z "$OUTPUT_FILE" ]; then
  echo "Usage: $0 <num_peers> <output_file> [algorithm"
  echo "Algorithms: bracha, afrt, codedmbrb"
  exit 1
fi

cat <<EOF > $OUTPUT_FILE
general:
  stop_time: 10s

network:
  graph:
    type: 1_gbit_switch

hosts:
  controller:
    network_node_id: 0
    processes:
    - path: ../bin/controller
      args: "-addr=controller:9000 -targetNodes=$NUM_PEERS -algorithm=$ALGORITHM -payloadSize=$PAYLOADSIZE"
      start_time: 0s 
      expected_final_state: running
EOF


for (( i=0; i<$NUM_PEERS; i++ ))
do
  cat <<EOF >> $OUTPUT_FILE

  peer$i:
    network_node_id: 0
    processes:
    - path: ../bin/node
      args: "-id=$i -addr=peer$i:8080 -controller=controller:9000"
      start_time: 1s
      expected_final_state: {exited: 0}
EOF
done
