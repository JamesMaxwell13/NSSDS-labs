package main

import (
	"fmt"
	"lab_2/client"
	"lab_2/server"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("use flag -c for running client or -s for running server")
		os.Exit(1)
	}

	mode := os.Args[1]
	switch mode {
	case "-s":
		s := new(server.Server)
		s.RunServer()
	case "-c":
		c := new(client.Client)
		c.RunClient()
	default:
		fmt.Printf("unknown argument: %s\n", mode)
		os.Exit(1)
	}

}
