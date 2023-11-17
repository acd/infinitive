package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/acd/infinitive/infinity"
	log "github.com/sirupsen/logrus"
)

func main() {
	httpPort := flag.Int("httpport", 8080, "HTTP port to listen on")
	serialPort := flag.String("serial", "", "path to serial port")

	flag.Parse()

	if len(*serialPort) == 0 {
		fmt.Print("must provide serial\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.SetLevel(log.DebugLevel)

	infinityApi, err := infinity.NewApi(context.Background(), *serialPort)
	if err != nil {
		log.Panicf("error opening serial port: %s", err.Error())
	}

	launchWebserver(*httpPort, infinityApi)
}
