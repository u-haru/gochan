package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/u-haru/gochan"
)

var (
	Dir  = ""
	Host = ""
)

func main() {
	flag.StringVar(&Dir, "d", "./Server", "-d [ServerDir]")
	flag.StringVar(&Host, "h", "0.0.0.0:80", "-h [Host]")
	flag.Parse()

	Server := gochan.NewServer(Dir)
	Server.Host = Host
	Server.Function.MessageChecker = messageChecker

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go canseler(c, Server.Saver)
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

func messageChecker(res *gochan.Res) (bool, string) {
	if strings.Contains(res.Message, "ハゲ") {
		return false, "ハゲじゃねえわ"
	}
	res.Message = strings.ReplaceAll(res.Message, "test", "テスト")
	return true, ""
}
