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
	subject := escape.Replace(toUTF(r.PostFormValue("subject")))
	from := escape.Replace(toUTF(r.PostFormValue("FROM")))
	mail := escape.Replace(toUTF(r.PostFormValue("mail")))
	bbs := toUTF(r.PostFormValue("bbs"))
	key := toUTF(r.PostFormValue("key"))
	now := time.Now()
	message := escape.Replace(toUTF(r.PostFormValue("MESSAGE")))

	if board, ok := sv.Boards[bbs]; !ok {
		dispError(w, "bbsが不正です!")
		return
	} else {
		if subject != "" { //subjectがあれば新規スレ
			key = fmt.Sprintf("%d", now.Unix())
			if uint(len(subject)) > board.Config.subjectMaxLen {
				dispError(w, "タイトルが長すぎます!")
				return
			}
			if _, ok := board.Threads[key]; ok { //すでに同じキーのスレがあったら
				dispError(w, "keyが不正です!")
				return
			}
			board.InitThread(key)
			if v, ok := board.Threads[key]; ok {
				v.Title = subject
			}
		} else {
			if _, ok := board.Threads[key]; !ok {
				dispError(w, "keyが不正です!")
				return
			}
		}
		if from == "" {
			from = board.Config.noName
		}
		if uint(len(message)) > board.Config.messageMaxLen {
			dispError(w, "本文が長すぎます!")
			return
		}
		if message == "" {
			dispError(w, "本文が空です!")
			return
		}

		id := GenerateID(r.RemoteAddr)      // ID生成
		if sv.Function.IDGenerator != nil { // もしID生成器が別で指定されていれば
			id = sv.Function.IDGenerator(r.RemoteAddr)
		}
		date_id := strings.Replace(now.Format("2006-01-02(<>) 15:04:05.00"), "<>", wdays[now.Weekday()], 1) + " ID:" + string(id) // 2021-08-25(水) 22:44:30.40 ID:MgUxkbjl0
		outdat := from + "<>" + mail + "<>" + date_id + "<>" + message + "<>" + subject + "\n"                                    // 吐き出すDat

		if sv.Function.MessageChecker != nil {
			if ok, reason := sv.Function.MessageChecker(from, mail, message, subject); !ok {
				dispError(w, reason)
				return
			}
		}
		if board.Threads[key].num >= board.Config.threadMaxRes {
			dispError(w, "このスレッドは"+fmt.Sprint(board.Config.threadMaxRes)+"を超えました。\n新しいスレッドを立ててください。")
			return
		} else {
			board.Threads[key].lock.Lock()
			board.Threads[key].Dat += outdat
			board.Threads[key].num++
			board.Threads[key].lastmod = now
			board.Threads[key].lock.Unlock()
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
	// subjects := map[string]string{} //マップ
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

	tmp := ""
	for _, k := range subs {
		tmp += k.key + ".dat<>" + k.title + "\n"
	}

	bd.Subject = tmp
}

// 8バイトのランダムな値+1バイトの"0"を返す
func GenerateID(remote string) []byte {
	now := time.Now()
	ip := strings.Split(remote, ":")[0] + now.Format("20060102")
	h := md5.New()
	io.WriteString(h, ip) //ip to md5

	seed := int64(binary.BigEndian.Uint64(h.Sum(nil)))
	rn := rand.New(rand.NewSource(seed)) //create local rand

	const rs2Letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+/"

	b := make([]byte, 9) //id[8] + 末尾
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
			fmt.Fprint(w, toSJIS(val.Dat))
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

	sv.reloadSettings(bbs)
	sv.saveSettings(bbs)
}

func (sv *server) DeleteBoard(bbs string) {
	os.Remove(sv.Dir + "/" + bbs)
	if _, ok := sv.Boards[bbs]; ok {
		delete(sv.Boards, bbs)
	}
}

func (bd *board) DeleteThread(bbs, key string) {
	os.Remove(bd.server.Dir + "/" + bbs + "/dat/" + key + ".dat")
	if _, ok := bd.Threads[key]; ok {
		delete(bd.Threads, key)
	}
}

func (th *thread) DeleteRes(bbs string, key string, num int) {
	tmp := strings.Split(th.Dat, "\n")
	if len(tmp) >= num {
		targetres := tmp[num-1]
		tmp := strings.Split(targetres, "<>")
		replaceres := "あぼーん<>" + tmp[1] + "<>" + tmp[2] + "<>あぼーん<>" + tmp[4]
		strings.Replace(th.Dat, targetres, replaceres, 1)
	}
}
