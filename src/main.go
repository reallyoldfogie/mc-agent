package main

import (
	"flag"
	"fmt"

	_ "github.com/Tnze/go-mc/data/lang/en-us"
)

var (
	serverAddr string
	serverPort uint64
	credFile   string
)

func main() {
	flag.StringVar(&serverAddr, "server", "localhost", "The minecraft server to connect to")
	flag.Uint64Var(&serverPort, "port", 25565, "The minecraft port to connect to.")
	flag.StringVar(&credFile, "credfile", "credentials.json", "The file containing minecraft credentials the bot will login with")

	flag.Parse()

	addr := fmt.Sprintf("%s:%d", serverAddr, serverPort)
	JoinServer(addr, credFile)

}
