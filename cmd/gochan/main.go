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
	Dir  = ""
	Host = ""
)

func main() {
	flag.StringVar(&Dir, "d", "./Server", "-d [ServerDir]")
	flag.StringVar(&Host, "h", "0.0.0.0:80", "-h [Host]")
	flag.Parse()

	// Server := gochan.NewServer(Dir)
	Server := &gochan.Server{}
	Server.Dir = Dir
	Server.Host = Host
	Server.SetLocation("Asia/Tokyo")
	Server.Function.MessageChecker = messageChecker

	Server.Init()

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
	// if strings.Contains(res.Message, "ハゲ") {
	// 	return false, "ハゲじゃねえわ"
	// }
	// res.Message = strings.ReplaceAll(res.Message, "test", "テスト")

	f, err := os.OpenFile("access.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		_, err := f.WriteString(fmt.Sprintf("[%s] %s/%s(Title: %s)\n\t%d.%s %s(Mail: %s) :%s\n\tHost:%s\tUA:%s\n",
			res.Date.Format("2006-01-02 15:04:05.00"),
			res.Thread().Board().BBS(),
			res.Thread().Key(),
			res.Thread().Title(),
			res.Thread().Num()+1,
			res.From, res.ID,
			res.Mail, res.Message,
			res.Log.Host, res.Log.UA))
		if err != nil {
			log.Println(err)
		}
		f.Close()
	}

	return true, ""
}
