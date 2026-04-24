PEERS ?= 4
ALGORITHM ?= codedmbrb
PAYLOADSIZE ?= 1024
T ?= 0
D ?= 0

build:
	@mkdir -p ./bin
	go build -o ./bin/node ./cmd/node
	go build -o ./bin/controller ./cmd/controller

clean:
	rm -rf ./bin
	rm -rf ./evaluations/sim

sim: clean build
	@mkdir -p ./evaluations/sim
	./evaluations/scripts/sim/generate_shadow_config.sh $(PEERS) ./evaluations/sim/shadow.yaml $(ALGORITHM) $(PAYLOADSIZE) $(T) $(D)
	cd ./evaluations/sim && shadow shadow.yaml
