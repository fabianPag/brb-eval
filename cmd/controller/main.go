package main

import (
	"brb-eval/internal/controller"
	"flag"
	"fmt"
	"os"
)

func main() {
	listenAddr := flag.String("addr", "", "Listen address")
	targetNodes := flag.Int("targetNodes", -1, "Amount of nodes before allowing p2p network start")
	tFlag := flag.Int("t", -1, "Number of malicious nodes")
	dFlag := flag.Int("d", 0, "Number of nodes affected by message adversary")
	algorithmFlag := flag.String("algorithm", "codembrb", "Algorithm used (bracha, afrt, codedmbrb)")
	flag.Parse()
	if *targetNodes <= 0 || *listenAddr == "" {
		fmt.Println("Usage: controller -addr=<host:port> -targetNodes=<num> [-t=<num>] [-d=<num>] [-algo=<name>]")
		os.Exit(1)
	}

	t := *tFlag
	d := *dFlag
	if t == -1 {
		t = (*targetNodes - 2*d - 1) / 3
	}

	contr := controller.New(*targetNodes, t, d, *algorithmFlag)
	err := contr.Run(*listenAddr)
	if err != nil {
		fmt.Println("Failed to start controller:", err)
		os.Exit(1)
	}
}
