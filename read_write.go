package gochan

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var escape = strings.NewReplacer(
	"\r\n", "<br>",
	"\r", "<br>",
	"\n", "<br>",
	"\"", "&quot;",
	"<", "&lt;",
	">", "&gt;",
)

var wdays = []string{"日", "月", "火", "水", "木", "金", "土"}

func (sv *server) bbs(w http.ResponseWriter, r *http.Request) { //bbs.cgiと同じ動きする
	bbs := toUTF(r.PostFormValue("bbs"))
	key := toUTF(r.PostFormValue("key"))

	res := &Res{}
	res.Subject = escape.Replace(toUTF(r.PostFormValue("subject")))
	res.From = escape.Replace(toUTF(r.PostFormValue("FROM")))
	res.Mail = escape.Replace(toUTF(r.PostFormValue("mail")))
	res.Message = escape.Replace(toUTF(r.PostFormValue("MESSAGE")))
	res.Date = time.Now()

	if board, ok := sv.Boards[bbs]; !ok {
		dispError(w, "bbsが不正です!")
		return
	} else {
		if res.Subject != "" { //subjectがあれば新規スレ
			key = fmt.Sprintf("%d", res.Date.Unix())
			if uint(len(res.Subject)) > board.Config.subjectMaxLen {
				dispError(w, "タイトルが長すぎます!")
				return
			}
			if _, ok := board.Threads[key]; ok { //すでに同じキーのスレがあったら
				dispError(w, "keyが不正です!")
				return
			}
			board.NewThread(key)
			if v, ok := board.Threads[key]; ok {
				v.Title = res.Subject
			}
		} else {
			if _, ok := board.Threads[key]; !ok {
				dispError(w, "keyが不正です!")
				return
			}
		}
		if res.From == "" {
			res.From = board.Config.noName
		}
		if uint(len(res.Message)) > board.Config.messageMaxLen {
			dispError(w, "本文が長すぎます!")
			return
		}
		if res.Message == "" {
			dispError(w, "本文が空です!")
			return
		}

		if sv.Function.IDGenerator != nil { // もしID生成器が別で指定されていれば
			res.ID = sv.Function.IDGenerator(r.RemoteAddr)
		} else {
			res.ID = GenerateID(r.RemoteAddr) // ID生成
		}

		if sv.Function.MessageChecker != nil {
			if ok, reason := sv.Function.MessageChecker(res); !ok {
				dispError(w, reason)
				return
			}
		}

		if board.Threads[key].num >= board.Config.threadMaxRes {
			dispError(w, "このスレッドは"+fmt.Sprint(board.Config.threadMaxRes)+"を超えました。\n新しいスレッドを立ててください。")
			return
		} else {
			board.Threads[key].NewRes(res)
		}

		w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
		w.Write([]byte(toSJIS(`<html>
		<head>
		<title>書きこみました。</title>
		<meta http-equiv="refresh" content="1;URL=` + "/" + bbs + "/?key=" + key + `">
		</head>
		<body>書きこみが終わりました。<br>
		画面を切り替えるまでしばらくお待ち下さい。
		</body>
		</html>`)))
		board.refresh_subjects()
	}
}

func (bd *board) refresh_subjects() {
	type str struct {
		key     string
		title   string
		lastmod time.Time
	}
	var subs []str

	for i, v := range bd.Threads {
		subs = append(subs, str{
			key:     i,
			title:   v.Title,
			lastmod: v.lastmod,
		})
	}

	sort.Slice(subs, func(i, j int) bool {
		return subs[i].lastmod.After(subs[j].lastmod)
	}) // ソート

	bd.Subject = ""
	for _, k := range subs {
		bd.Subject += k.key + ".dat<>" + k.title + "\n"
	}
}

// 8バイトのランダムな値+1バイトの"0"を返す
func GenerateID(remote string) [9]byte {
	now := time.Now()
	ip := strings.Split(remote, ":")[0] + now.Format("20060102")
	h := md5.New()
	io.WriteString(h, ip) //ip to md5

	seed := int64(binary.BigEndian.Uint64(h.Sum(nil)))
	rn := rand.New(rand.NewSource(seed)) //create local rand

	const rs2Letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+/"

	b := [9]byte{}
	for i := 0; i < 8; i++ {
		b[i] = rs2Letters[rn.Intn(len(rs2Letters))]
	}
	b[8] = '0'

	return b
}

func (sv *server) dat(w http.ResponseWriter, r *http.Request) { //dat
	path := r.URL.Path[1:]
	path = strings.TrimSuffix(path, "/")
	strs := strings.Split(path, "/")
	if len(strs) < 3 {
		dispError(w, "Bad Request!")
		return
	}
	bbs := strs[0]
	dotpos := strings.LastIndex(strs[2], ".dat")
	if dotpos < 0 {
		dispError(w, "keyが不正です!")
		return
	}
	key := strs[2][:dotpos]

	if val, ok := sv.Boards[bbs]; ok {
		if val, ok := val.Threads[key]; ok {
			w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
			val.lock.RLock()
			fmt.Fprint(w, toSJIS(val.dat))
			val.lock.RUnlock()
		}
	}
}

func (sv *server) sub(w http.ResponseWriter, r *http.Request) { //subject.txt
	path := r.URL.Path[1:]
	path = strings.TrimSuffix(path, "/")
	bbs := strings.Split(path, "/")[0]
	w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
	if bd, ok := sv.Boards[bbs]; ok {
		stream_toSJIS(strings.NewReader(bd.Subject), w)
	} else {
		dispError(w, "bbsが不正です!")
	}
}

func dispError(w http.ResponseWriter, stat string) {
	w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
	title := "ERROR!"
	body := escape.Replace(toSJIS(stat))
	fmt.Fprint(w, `<head>
	<title>`+title+`</title>
	</head>
	<body>`+body+`
	</body>
	</html>`)
}

func readalltxt(path string) string {
	file, err := os.Open(path)
	if err != nil {
		log.Println(err)
		return ""
	}
	tmp, _ := ioutil.ReadAll(file)
	file.Close()
	return string(tmp)
}

func (sv *server) NewBoard(bbs, title string) {
	if !exists(sv.Dir + "/" + bbs) {
		os.MkdirAll(sv.Dir+"/"+bbs+"/dat/", 755)
	}
	bd := sv.InitBoard(bbs)
	bd.Config.Raw["BBS_TITLE"] = title
	bd.Config.Raw["BBS_TITLE_ORIG"] = title
	bd.Config.Raw["BBS_NONAME_NAME"] = "名無しさん"
	bd.Config.Raw["BBS_DELETE_NAME"] = "あぼーん"
	bd.Config.Raw["BBS_MAX_RES"] = "1000"
	bd.Config.Raw["BBS_MESSAGE_MAXLEN"] = "2048"
	bd.Config.Raw["BBS_SUBJECT_MAXLEN"] = "30"

	sv.Boards[bbs].reloadSettings()
	sv.Boards[bbs].saveSettings()
}

func (sv *server) DeleteBoard(bbs string) {
	os.Remove(sv.Dir + "/" + bbs)
	if _, ok := sv.Boards[bbs]; ok {
		delete(sv.Boards, bbs)
	}
}

func (bd *board) NewThread(key string) *thread {
	th := &thread{}
	th.lock = sync.RWMutex{}
	bd.Threads[key] = th
	return th
}

func (bd *board) DeleteThread(bbs, key string) {
	os.Remove(bd.server.Dir + "/" + bbs + "/dat/" + key + ".dat")
	if _, ok := bd.Threads[key]; ok {
		delete(bd.Threads, key)
	}
}

func (th *thread) NewRes(res *Res) {
	date_id := strings.Replace(res.Date.Format("2006-01-02(<>) 15:04:05.00"), "<>", wdays[res.Date.Weekday()], 1) + " ID:" + string(res.ID[:]) // 2021-08-25(水) 22:44:30.40 ID:MgUxkbjl0
	outdat := res.From + "<>" + res.Mail + "<>" + date_id + "<>" + res.Message + "<>" + res.Subject + "\n"                                     // 吐き出すDat
	th.lock.Lock()
	th.dat += outdat
	th.num++
	th.lastmod = res.Date
	th.lock.Unlock()
}

func (th *thread) DeleteRes(num int) {
	tmp := strings.Split(th.dat, "\n")
	if len(tmp) >= num {
		targetres := tmp[num-1]
		tmp := strings.Split(targetres, "<>")
		replaceres := "あぼーん<>" + tmp[1] + "<>" + tmp[2] + "<>あぼーん<>" + tmp[4]
		strings.Replace(th.dat, targetres, replaceres, 1)
	}
}
