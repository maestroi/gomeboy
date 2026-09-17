package main

import (
	"flag"
	"log"
	"net"

	"github.com/maestroi/gomeboy/pkg/link"
)

func main() {
	listen := flag.String("listen", ":8765", "TCP address to listen on")
	flag.Parse()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	log.Printf("gomeboy link broker listening on %s", ln.Addr())
	if err := link.NewBroker().Serve(ln); err != nil {
		log.Fatal(err)
	}
}
