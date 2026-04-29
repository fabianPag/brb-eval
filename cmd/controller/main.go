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
	tFlag := flag.Int("t", -1, "The t value in the n > 3t + 2d formula (-1 to infer max, requires d to be set)")
	dFlag := flag.Int("d", 0, "The d value in the n > 3t + 2d formula (-1 to infer max, requires t to be set)")
	algorithmFlag := flag.String("algorithm", "codedmbrb", "Algorithm used (bracha, afrt, codedmbrb)")
	numSilent := flag.Int("numSilent", 0, "Number of nodes to be Byzantine silent")
	numIsolated := flag.Int("numIsolated", 0, "Number of nodes to isolate simulating Message Adversary")
	payloadSize := flag.Int("payloadSize", 1000, "Size of payload to broadcast in bytes")
	saveMetrics := flag.Bool("saveMetrics", true, "Should collected metrics be saved to file?")
	twinsSender := flag.Bool("twinsSender", false, "Enable Twins sender mode")

	flag.Parse()
	if *targetNodes <= 0 || *listenAddr == "" {
		fmt.Println("Usage: controller -addr=<host:port> -targetNodes=<num> [-t=<num>] [-d=<num>] [-algo=<name>] [-payloadSize=<bytes>] [-saveMetrics=<true|false>]")
		os.Exit(1)
	}

	t := *tFlag
	d := *dFlag

	if t == -1 && d == -1 {
		fmt.Println("Cannot infer both t and d. Please provide at least one explicit threshold.")
		os.Exit(1)
	}

	if t == -1 {
		t = max((*targetNodes-2*d-1)/3, 0)
		fmt.Printf("Inferred max Byzantine threshold: t=%d\n", t)
	}

	if d == -1 {
		d = max((*targetNodes-3*t-1)/2, 0)
		fmt.Printf("Inferred max Message Adversary threshold: d=%d\n", d)
	}

	if 3*t+2*d >= *targetNodes {
		fmt.Println("Invalid configuration: 3t + 2d must be strictly less than n. Got t=%d, d=%d, n=%d", t, d, *targetNodes)
		os.Exit(1)
	}

	contr := controller.New(*targetNodes, t, d, *algorithmFlag, *payloadSize, *twinsSender, *numSilent, *numIsolated)
	err := contr.Run(*listenAddr, *saveMetrics)
	if err != nil {
		fmt.Println("Failed to start controller:", err)
		os.Exit(1)
	}
}
