N ?= 4
ALGORITHM ?= codedmbrb
PAYLOADSIZE ?= 1KB
T ?= -1
D ?= 0
TWINSSENDER ?= false
NUMSILENT ?= 0
NUMMSGDROPS ?= 0
EXPECTED_FINAL_STATE ?= 0

.PHONY: build clean sim sim-fast

build:
	@mkdir -p ./bin
	go build -o ./bin/node ./cmd/node
	go build -o ./bin/controller ./cmd/controller

clean:
	rm -rf ./bin
	rm -rf ./evaluations/sim

sim: build sim-fast

sim-fast:
	@mkdir -p ./evaluations/sim
	@rm -rf ./evaluations/sim/*
	./evaluations/scripts/sim/generate_shadow_config.sh $(N) ./evaluations/sim/shadow.yaml $(ALGORITHM) $(PAYLOADSIZE) $(T) $(D) $(TWINSSENDER) $(NUMSILENT) $(NUMMSGDROPS) $(EXPECTED_FINAL_STATE)
	cd ./evaluations/sim && shadow shadow.yaml