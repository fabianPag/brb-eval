package main

import (
	"brb-eval/internal/controller"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	listenAddr := flag.String("addr", "", "Listen address")
	targetNodes := flag.Int("targetNodes", -1, "Amount of nodes before allowing p2p network start")
	tFlag := flag.Int("t", -1, "The t value in the n > 3t + 2d formula (-1 to infer max, requires d to be set)")
	dFlag := flag.Int("d", 0, "The d value in the n > 3t + 2d formula (-1 to infer max, requires t to be set)")
	algorithmFlag := flag.String("algorithm", "codedmbrb", "Algorithm used (bracha, afrt, codedmbrb)")
	numSilent := flag.Int("numSilent", 0, "Number of nodes to be Byzantine silent")
	numMsgDrops := flag.Int("numMsgDrops", 0, "Number of messages dropped by the Message Adversary per broadcast")
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
	algorithm := strings.ToLower(*algorithmFlag)

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
		fmt.Printf("Invalid configuration: 3t + 2d must be strictly less than n. Got t=%d, d=%d, n=%d\n", t, d, *targetNodes)
		os.Exit(1)
	}

	if algorithm != "bracha" && algorithm != "afrt" && algorithm != "codedmbrb" {
		fmt.Printf("Invalid algorithm: %s. Supported algorithms are: bracha, afrt, codedmbrb\n", *algorithmFlag)
		os.Exit(1)
	}

	if *payloadSize <= 0 {
		fmt.Printf("Invalid payload size: %d. Must be positive.\n", *payloadSize)
		os.Exit(1)
	}

	// The sender will never be silent, its either honest or twins_sender
	if *numSilent < 0 || *numSilent > *targetNodes-1 {
		fmt.Printf("Invalid numSilent: %d. Must be between 0 and %d (n-1).\n", *numSilent, *targetNodes-1)
		os.Exit(1)
	}

	// Adversary only targets honest nodes, and it can't drop messages from a node to itself, so the max possible drops is n - numSilent - 1.
	maxPossibleDrops := max(*targetNodes-*numSilent-1, 0)

	if *numMsgDrops < 0 || *numMsgDrops > maxPossibleDrops {
		fmt.Printf("Invalid numMsgDrops (%d): Only %d honest external targets available (with %d silent nodes).\n", *numMsgDrops, maxPossibleDrops, *numSilent)
		os.Exit(1)
	}

	contr := controller.New(*targetNodes, t, d, algorithm, *payloadSize, *twinsSender, *numSilent, *numMsgDrops)
	err := contr.Run(*listenAddr, *saveMetrics)
	if err != nil {
		fmt.Println("Failed to start controller:", err)
		os.Exit(1)
	}
}
