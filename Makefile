PEERS ?= 4
ALGORITHM ?= codedmbrb
PAYLOADSIZE ?= 1024

build:
	@mkdir -p ./bin
	go build -o ./bin/node ./cmd/node
	go build -o ./bin/controller ./cmd/controller

clean:
	rm -rf ./bin
	rm -rf ./sim

sim: clean build
	@mkdir -p ./sim
	./scripts/generate_shadow_config.sh $(PEERS) sim/shadow.yaml $(ALGORITHM) $(PAYLOADSIZE)
	cd ./sim && shadow shadow.yaml
