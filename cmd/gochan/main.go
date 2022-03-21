package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/u-haru/gochan"
	"github.com/u-haru/gochan/pkg/admin"
)

var (
	Dir  = ""
	Host = ""
)

func main() {
	flag.StringVar(&Dir, "d", "./Server", "-d [ServerDir]")
	flag.StringVar(&Host, "h", "0.0.0.0:80", "-h [Host]")
	flag.Parse()

	Server := gochan.NewServer()
	Server.Function.WriteChecker = messageChecker
	Server.Function.ArchiveChecker = archiveChecker
	Server.Function.RuleGenerator = RuleGenerator
	// Server.Baseurl = "/a/"

	ab := &admin.Board{
		Server: Server,
		Path:   "/admin/",
		Hash:   "noauth",
	}
	Server.HTTPServeMux.Handle(ab.Path, ab)

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		s := <-c
		fmt.Printf("Signal received: %s \n", s.String())
		Server.Save()
		close(c)
		os.Exit(130)
	}()
	go func() {
		for {
			for _, b := range Server.Boards() {
				b.Squash()
			}
			<-time.After(time.Second * 10)
		}
	}()

	log.Println("Listening on: " + Host)
	log.Println(Server.ListenAndServe(Host, Dir))
}

var list struct {
	sync.RWMutex
	messager map[string]time.Time
	sync.Once
}

var triplist map[string]string = map[string]string{
	"c045526b5ddad91b2f0d13168590f19a7113e347d7681a673ea308aa7dee2f09": "管理人",
}

func messageChecker(res *gochan.Res) (bool, string) {
	list.Do(func() {
		go func() {
			for {
				<-time.After(time.Second * 10)
				now := time.Now()
				for s, v := range list.messager {
					if v.Add(time.Second * 10).After(now) {
						list.Lock()
						delete(list.messager, s)
						list.Unlock()
					}
				}
				// log.Println("Wiped")
			}
		}()
	})
	// if strings.Contains(res.Message, "ハゲ") {
	// 	return false, "ハゲじゃねえわ"
	// }
	// res.Message = strings.ReplaceAll(res.Message, "test", "テスト")

	if c, err := res.Req.Cookie("AcceptRule"); err != nil || c.Value != "true" {
		http.SetCookie(res.Writer, &http.Cookie{
			Name:    "AcceptRule",
			Value:   "true",
			Path:    res.Thread().Board().BBS(),
			Expires: time.Now().Add(time.Hour * 24),
		})
		return false, "書き込んでもよろしいですか?\n書き込みに対し本サイトはいかなる責任も負いません。今後行われた書き込みに対しては、この規約に同意したものとみなします。\n書き込みを行う場合はページを再読み込みしてください。"
	}

	v, ok := list.messager[strings.Split(res.Req.RemoteAddr, ":")[0]]
	if ok {
		if v.Add(time.Second * 5).After(res.Date) { //前回の書き込みから5秒以内
			return false, "マルチポストですか?"
		}
	}
	list.Lock()
	if list.messager == nil {
		list.messager = make(map[string]time.Time)
	}
	list.messager[strings.Split(res.Req.RemoteAddr, ":")[0]] = res.Date
	list.Unlock()

	res.From = strings.ReplaceAll(res.From, "★", "☆")
	pos := strings.Index(res.From, "#")
	if pos != -1 {
		if n, ok := triplist[admin.Hash(res.From[pos:])]; ok {
			res.From = n
		} else {
			trip := admin.Hash(res.From[pos:])
			trip = strings.ToUpper(trip[len(trip)-6:])
			res.From = fmt.Sprintf("%s★%s", res.From[:pos], trip)
		}
	} else {
		for _, n := range triplist {
			if strings.Contains(res.From, n) {
				res.From, _ = res.Thread().Conf.GetString("NONAME")
				break
			}
		}
	}

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
			res.Req.RemoteAddr, res.Req.UserAgent()))
		if err != nil {
			log.Println(err)
		}
		f.Close()
	}

	return true, ""
}

func archiveChecker(th *gochan.Thread) bool {
	if !th.Writable() {
		if th.Lastmod().Add(time.Minute * 2).Before(time.Now()) { //スレ落ちから2分経過
			return true
		}
	}
	return false
}

var noname = regexp.MustCompile("NONAME=(.*)<br>")

func RuleGenerator(th *gochan.Thread) {
	res1, err := th.GetRes(1) //1つめの書き込みゲット
	if err != nil {
		return
	}
	group := noname.FindSubmatch([]byte(res1.Message))
	if len(group) == 2 {
		th.Conf.Set("NONAME", string(group[1]))
	}
}
