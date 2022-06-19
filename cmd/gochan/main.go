package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/u-haru/gochan"
	"github.com/u-haru/gochan/pkg/admin"
)

var (
	Dir  = ""
	Host = ""
	rkey *rsa.PrivateKey
)

func main() {
	flag.StringVar(&Dir, "d", "./Server", "-d [ServerDir]")
	flag.StringVar(&Host, "h", "0.0.0.0:80", "-h [Host]")
	flag.Parse()

	rkey, _ = rsa.GenerateKey(rand.Reader, 1024)

	Server := gochan.NewServer()
	Server.Function.WriteChecker = messageChecker
	Server.Function.ArchiveChecker = archiveChecker
	Server.Function.RuleGenerator = RuleGenerator
	Server.Description = "gochan@u-haru.com"
	// Server.Baseurl = "/a/"

	ab := &admin.Board{
		Server: Server,
		Path:   "/admin/",
		Hash:   "noauth",
	}
	Server.Handle(ab.Path, ab)

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		s := <-c
		fmt.Printf("Signal received: %s \n", s.String())
		Server.Save()
		close(c)
		os.Exit(0)
	}()

	log.Println("Listening on: " + Host)
	log.Println(Server.ListenAndServe(Host, Dir))
}

var list struct {
	sync.RWMutex
	messager map[string]time.Time
	sync.Once
}

type user struct {
	Name, ID string
}

var userlist map[string]user = map[string]user{
	"c045526b5ddad91b2f0d13168590f19a7113e347d7681a673ea308aa7dee2f09": {"管理人", "ADMIN"},
	"3c46148eb25bea277ae350d6588f36e292da6a3e9ca73e4d915bf432f31f3369": {"システム", "SYSTEMUSER"},
}
var domains = make(map[string]int)
var domainmod = false

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
				if domainmod {
					f, err := os.OpenFile("domains.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
					if err == nil {
						b, err := json.Marshal(domains)
						if err == nil {
							f.Write(b)
						}
						f.Close()
					}
					domainmod = false
				}
				// log.Println("Wiped")
			}
		}()
	})
	// if strings.Contains(res.Message, "ハゲ") {
	// 	return false, "ハゲじゃねえわ"
	// }
	// res.Message = strings.ReplaceAll(res.Message, "test", "テスト")
	th := res.Thread()

	var needauth = true
	c, err := res.Req.Cookie("AcceptRule")
	if err == nil {
		mess, _ := base64.StdEncoding.DecodeString(c.Value)
		decryptedBytes, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, rkey, mess, nil)
		if err == nil && strings.Contains(string(decryptedBytes), "@gochan") {
			needauth = false
		}
	}

	if needauth {
		encryptedBytes, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &rkey.PublicKey, []byte("@gochan"), nil)
		if err != nil {
			log.Println(err)
		}
		mess := base64.StdEncoding.EncodeToString(encryptedBytes)
		http.SetCookie(res.Writer, &http.Cookie{
			Name:    "AcceptRule",
			Value:   mess,
			Path:    th.Board().BBS(),
			Expires: time.Now().Add(time.Hour * 24),
		})
		list.Lock()
		if list.messager == nil {
			list.messager = make(map[string]time.Time)
		}
		list.messager[res.RemoteAddr.String()] = res.Date.Add(-time.Second * 2)
		list.Unlock()
		return false, `書き込んでもよろしいですか?<br><br>書き込みに対し本サイトはいかなる責任も負いません。今後行われた書き込みに対しては、この規約に同意したものとみなします。<br>
書き込みは3秒後から有効になります。
<form method="POST" accept-charset="Shift-JIS">
<input type="submit" value="書き込む"><br>
<input type="hidden" name="FROM" value="` + res.From + `">
<input type="hidden" name="mail" value="` + res.Mail + `">
<input type="hidden" name="MESSAGE" value="` + res.Message + `">
<input type="hidden" name="bbs" value="` + res.BBS + `">
<input type="hidden" name="key" value="` + res.Key + `"></form>`
	}

	v, ok := list.messager[res.RemoteAddr.String()]
	if ok {
		if v.Add(time.Second * 5).After(res.Date) { //前回の書き込みから5秒以内
			return false, "マルチポストですか?"
		}
	}
	list.Lock()
	if list.messager == nil {
		list.messager = make(map[string]time.Time)
	}
	list.messager[res.RemoteAddr.String()] = res.Date
	list.Unlock()

	res.From = strings.ReplaceAll(res.From, "★", "☆")
	res.From = strings.Replace(res.From, "fusianasan", res.RemoteAddr.String(), 1)

	pos := strings.Index(res.From, "#")
	wf := false //管理者の書き込み
	if pos != -1 {
		if u, ok := userlist[admin.Hash(res.From[pos:])]; ok {
			res.From = u.Name
			res.ID = []byte(u.ID)
			wf = true
		} else {
			trip := admin.Hash(res.From[pos:])
			trip = strings.ToUpper(trip[len(trip)-6:])
			res.From = fmt.Sprintf("%s★%s", res.From[:pos], trip)
		}
	} else {
		for _, u := range userlist {
			if strings.Contains(res.From, u.Name) {
				res.From, _ = th.Conf.GetString("NONAME")
				break
			}
		}
	}
	wable, err := th.Conf.GetBool("wable")
	if err == nil && !wable && !wf { //管理者以外書き込み禁止のスレ
		return false, "このスレには書き込めません"
	}

	f, err := os.OpenFile("access.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		_, err := f.WriteString(fmt.Sprintf("[%s] %s/%s(Title: %s)\n\t%d.%s %s(Mail: %s) :%s\n\tHost:%s\tUA:%s\n",
			res.Date.Format("2006-01-02 15:04:05.00"),
			th.Board().BBS(),
			th.Key(),
			th.Title(),
			th.Num()+1,
			res.From, res.ID,
			res.Mail, res.Message,
			res.RemoteAddr, res.Req.UserAgent()))
		if err != nil {
			log.Println(err)
		}
		f.Close()
	}
	go func(ip net.IP) {
		addr, err := net.LookupAddr(ip.String())
		if err == nil {
			domain := addr[0]
			tmp := strings.SplitAfterN(domain, ".", 2)
			if len(tmp) > 1 {
				domain = tmp[1]
			}
			domains[domain]++
		} else {
			domains[ip.String()]++
		}
		domainmod = true
	}(res.RemoteAddr)

	return true, ""
}

func archiveChecker(th *gochan.Thread, force bool) bool {
	aable, err := th.Conf.GetBool("aable")
	if err == nil && !aable {
		return false //アーカイブ出来ないスレは絶対にアーカイブしない(通知とか)
	}
	if !th.Writable() {
		if th.Lastmod().Add(time.Minute * 2).Before(time.Now()) { //スレ落ちから2分経過
			return true
		}
	}
	return force //スレ落ちして無くても強制なら落とす
}

var noname = regexp.MustCompile("NONAME=(.*?)($|<| )")

func RuleGenerator(th *gochan.Thread) {
	res1, err := th.GetRes(1) //1つめの書き込みゲット
	if err != nil {
		return
	}
	group := noname.FindSubmatch([]byte(res1.Message))
	if len(group) > 1 {
		th.Conf.Set("NONAME", string(group[1]))
	}
	if res1.From == "システム" {
		th.Conf.Set("aable", false) //システムからの書き込みはアーカイブ不可
		th.Conf.Set("wable", false) //システムからの書き込みは一般ユーザー書き込み不可
	}
}
