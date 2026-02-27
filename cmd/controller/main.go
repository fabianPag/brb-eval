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
	flag.Parse()
	if *targetNodes <= 0 || *listenAddr == "" {
		fmt.Println("Usage: controller -addr=<host:port> -targetNodes=<num>")
		os.Exit(1)
	}
	contr := controller.New(*targetNodes)
	err := contr.Run(*listenAddr)
	if err != nil {
		fmt.Println("Failed to start controller:", err)
		os.Exit(1)
	}
}
