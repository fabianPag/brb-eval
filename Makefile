PEERS ?= 4
ALGORITHM ?= codedmbrb
PAYLOADSIZE ?= 1000
T ?= -1
D ?= 0
TWINSSENDER ?= false
NUMSILENT ?= 0
NUMISOLATED ?= 0

build:
	@mkdir -p ./bin
	go build -o ./bin/node ./cmd/node
	go build -o ./bin/controller ./cmd/controller

clean:
	rm -rf ./bin
	rm -rf ./evaluations/sim

sim: clean build
	@mkdir -p ./evaluations/sim
	./evaluations/scripts/sim/generate_shadow_config.sh $(PEERS) ./evaluations/sim/shadow.yaml $(ALGORITHM) $(PAYLOADSIZE) $(T) $(D) $(TWINSSENDER) $(NUMSILENT) $(NUMISOLATED)
	cd ./evaluations/sim && shadow shadow.yaml
