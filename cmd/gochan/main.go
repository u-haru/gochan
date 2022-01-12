package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/u-haru/gochan"
)

var (
	Dir   = ""
	Host  = ""
	Noram = false
)

func main() {
	flag.StringVar(&Dir, "d", "./Server", "-d [ServerDir]")
	flag.StringVar(&Host, "h", "0.0.0.0:80", "-h [Host]")
	flag.BoolVar(&Noram, "n", false, "-n(Noram mode)")
	flag.Parse()

	Server := gochan.New(Dir)
	Server.Host = Host
	Server.Config.NoRam = Noram

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	if Server.Config.NoRam {
		go canseler(c, nil)
	} else {
		go canseler(c, Server.Saver)
	}
	log.Println("Listening on: " + Server.Host)
	// Server.NewBoard("test", "テスト")
	log.Println(Server.ListenAndServe())
}

func canseler(c chan os.Signal, exitfunc func()) {
	s := <-c
	fmt.Printf("Signal received: %s \n", s.String())
	if exitfunc != nil {
		exitfunc()
	}
	close(c)
	os.Exit(130)
}
