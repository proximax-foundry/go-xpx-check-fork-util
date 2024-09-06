package main

import (
	"flag"
	"log"
)

func main() {
	fileName := flag.String("file", "config.json", "Name of file to load config from")
	flag.Parse()

	config, err := LoadConfig(*fileName)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	fc, err := NewForkChecker(*config)
	if err != nil {
		log.Fatalf("Failed to setup fork checker: %v", err)
	}

	err = fc.Start()
	if err != nil {
		log.Fatalf("Error running fork checker: %v", err)
	}
}
