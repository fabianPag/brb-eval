build:
	@mkdir -p ../bin
	go build -o ../bin/brb-eval main.go

sim: build
	rm -rf ../sim/shadow.data
	cd ../sim && shadow shadow.yaml

clean:
	rm -rf ../bin
	rm -rf ../sim/shadow.data