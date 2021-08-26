package gochan

import (
	"flag"
	"fmt"
	"gochan/pkg/server"
	"os"
	"os/signal"
)

func Execute() {
	Dir := ""
	flag.StringVar(&Dir, "d", "./Server", "-d [ServerDir]")
	Host := ""
	flag.StringVar(&Host, "h", "0.0.0.0:80", "-h [Host]")
	Noram := false
	flag.BoolVar(&Noram, "n", false, "-n(Noram mode)")
	flag.Parse()

	Server := server.New(Dir)
	Server.Host = Host
	Server.Config.NoRam = Noram

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	if Server.Config.NoRam {
		go canseler(c, nil)
	} else {
		go canseler(c, Server.Saver)
	}
	Server.Start()
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
