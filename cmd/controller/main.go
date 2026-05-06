package main

import (
	"brb-eval/internal/controller"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func parsePayloadSize(s string) (int, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	var multiplier float64 = 1.0

	switch {
	case strings.HasSuffix(s, "MB"):
		multiplier = 1000 * 1000
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1000
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	default:
		return 0, fmt.Errorf("invalid payload size '%s': missing unit (B, KB, MB)", s)
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value '%s': %v", s, err)
	}

	if val <= 0 {
		return 0, fmt.Errorf("payload size must be positive, got %g", val)
	}

	return int(val * multiplier), nil
}

func main() {
	listenAddrFlag := flag.String("addr", "", "Listen address")
	nFlag := flag.Int("n", -1, "Total number of nodes (N) in the network")
	tFlag := flag.Int("t", -1, "The t value in the n > 3t + 2d formula (-1 to infer max, requires d to be set)")
	dFlag := flag.Int("d", 0, "The d value in the n > 3t + 2d formula (-1 to infer max, requires t to be set)")
	algorithmFlag := flag.String("algorithm", "codedmbrb", "Algorithm used (bracha, afrt, codedmbrb)")
	numSilentFlag := flag.Int("numSilent", 0, "Number of nodes to be Byzantine silent")
	numMsgDropsFlag := flag.Int("numMsgDrops", 0, "Number of messages dropped by the Message Adversary per broadcast")
	payloadSizeFlag := flag.String("payloadSize", "1KB", "Size of payload (e.g., 100B, 1KB, 2MB)")
	saveMetricsFlag := flag.Bool("saveMetrics", true, "Should collected metrics be saved to file?")
	twinsSenderFlag := flag.Bool("twinsSender", false, "Enable Twins sender mode")
	gracePeriodFlag := flag.Duration("gracePeriod", 3*time.Second, "Time to wait after all nodes are done before unblocking (e.g., 3s, 500ms, 0s)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: controller -addr=<host:port> -n=<num> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *nFlag <= 0 || *listenAddrFlag == "" {
		flag.Usage()
		os.Exit(1)
	}

	if *gracePeriodFlag < 0 {
		fmt.Printf("Invalid gracePeriod (%v): duration cannot be negative.\n", *gracePeriodFlag)
		os.Exit(1)
	}

	t := *tFlag
	d := *dFlag
	n := *nFlag
	algorithm := strings.ToLower(*algorithmFlag)

	payloadSize, err := parsePayloadSize(*payloadSizeFlag)
	if err != nil {
		fmt.Printf("Error parsing payload size: %v\n", err)
		os.Exit(1)
	}

	if t == -1 && d == -1 {
		fmt.Fprintf(os.Stderr, "Cannot infer both t and d. Please provide at least one explicit threshold.\n")
		os.Exit(1)
	}

	if t == -1 {
		t = max((n-2*d-1)/3, 0)
		fmt.Printf("Inferred max Byzantine threshold: t=%d\n", t)
	}

	if d == -1 {
		d = max((n-3*t-1)/2, 0)
		fmt.Printf("Inferred max Message Adversary threshold: d=%d\n", d)
	}

	if 3*t+2*d >= n {
		fmt.Fprintf(os.Stderr, "Invalid configuration: 3t + 2d must be strictly less than n. Got t=%d, d=%d, n=%d\n", t, d, n)
		os.Exit(1)
	}

	if algorithm != "bracha" && algorithm != "afrt" && algorithm != "codedmbrb" {
		fmt.Fprintf(os.Stderr, "Invalid algorithm: %s. Supported algorithms are: bracha, afrt, codedmbrb\n", *algorithmFlag)
		os.Exit(1)
	}

	// The sender will never be silent, its either honest or twins_sender
	if *numSilentFlag < 0 || *numSilentFlag > n-1 {
		fmt.Fprintf(os.Stderr, "Invalid numSilent: %d. Must be between 0 and %d (n-1).\n", *numSilentFlag, n-1)
		os.Exit(1)
	}

	// Adversary only targets honest nodes, and it can't drop messages from a node to itself
	maxPossibleDrops := max(n-*numSilentFlag-1, 0)

	if *numMsgDropsFlag < 0 || *numMsgDropsFlag > maxPossibleDrops {
		fmt.Fprintf(os.Stderr, "Invalid numMsgDrops (%d): Only %d honest external targets available (with %d silent nodes).\n", *numMsgDropsFlag, maxPossibleDrops, *numSilentFlag)
		os.Exit(1)
	}

	opts := controller.Options{
		N:                n,
		T:                t,
		D:                d,
		Algorithm:        algorithm,
		PayloadSize:      payloadSize,
		DesignatedSender: 0,
		TwinsSender:      *twinsSenderFlag,
		NumSilent:        *numSilentFlag,
		NumMsgDrops:      *numMsgDropsFlag,
		GracePeriod:      *gracePeriodFlag,
	}

	contr := controller.New(opts)

	err = contr.Run(*listenAddrFlag, *saveMetricsFlag)
	if err != nil {
		fmt.Println("Failed to start controller:", err)
		os.Exit(1)
	}
}
