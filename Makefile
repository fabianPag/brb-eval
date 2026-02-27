build:
	@mkdir -p ./bin
	go build -o ./bin/node ./cmd/node
	go build -o ./bin/controller ./cmd/controller

sim: build
	rm -rf ./sim/shadow.data
	cd ./sim && shadow shadow.yaml

clean:
	rm -rf ./bin
	rm -rf ./sim/shadow.data