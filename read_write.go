package gochan

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var Escape = strings.NewReplacer(
	"\r\n", "<br>",
	"\r", "<br>",
	"\n", "<br>",
	"\"", "&quot;",
	"<", "&lt;",
	">", "&gt;",
)

var wdays = []string{"日", "月", "火", "水", "木", "金", "土"}

func (sv *Server) bbs(w http.ResponseWriter, r *http.Request) { //bbs.cgiと同じ動きする
	bbs := toUTF(r.PostFormValue("bbs"))
	key := toUTF(r.PostFormValue("key"))

	res := &Res{}
	res.Subject = Escape.Replace(toUTF(r.PostFormValue("subject")))
	res.From = Escape.Replace(toUTF(r.PostFormValue("FROM")))
	res.Mail = Escape.Replace(toUTF(r.PostFormValue("mail")))
	res.Message = Escape.Replace(toUTF(r.PostFormValue("MESSAGE")))
	res.Date = time.Now()

	if board, ok := sv.boards[bbs]; !ok {
		dispError(w, "bbsが不正です!")
		return
	} else {
		if res.Subject != "" { //subjectがあれば新規スレ
			key = fmt.Sprintf("%d", res.Date.Unix())
			if uint(len(res.Subject)) > board.Config.subjectMaxLen {
				dispError(w, "タイトルが長すぎます!")
				return
			}
			if _, ok := board.threads[key]; ok { //すでに同じキーのスレがあったら
				dispError(w, "keyが不正です!")
				return
			}
			th := &Thread{}
			th.init(board, key)
			if v, ok := board.threads[key]; ok {
				v.title = res.Subject
			}
		} else {
			if _, ok := board.threads[key]; !ok {
				dispError(w, "keyが不正です!")
				return
			}
		}
		res.thread = board.threads[key]
		res.Req = *r
		res.Writer = w
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

		if sv.Function.WriteChecker != nil {
			if ok, reason := sv.Function.WriteChecker(res); !ok {
				dispError(w, reason)
				return
			}
		}

		if !board.threads[key].Writable() {
			dispError(w, "このスレッドは"+fmt.Sprint(board.Config.threadMaxRes)+"を超えました。\n新しいスレッドを立ててください。")
			return
		} else {
			board.threads[key].NewRes(res)
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

func (th *Thread) Writable() bool {
	return th.num < th.board.Config.threadMaxRes
}

func (bd *board) refresh_subjects() {
	type str struct {
		key     string
		title   string
		num     uint
		lastmod time.Time
	}
	subs := []str{}

	for i, v := range bd.threads {
		subs = append(subs, str{
			key:     i,
			title:   v.title,
			num:     v.num,
			lastmod: v.lastmod,
		})
	}

	sort.Slice(subs, func(i, j int) bool {
		return subs[i].lastmod.After(subs[j].lastmod)
	}) // ソート

	bd.subject = ""
	for _, k := range subs {
		bd.subject += fmt.Sprintf("%s.dat<>%s (%d)\n", k.key, k.title, k.num)
	}
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

	b := [9]byte{}
	for i := 0; i < 8; i++ {
		b[i] = rs2Letters[rn.Intn(len(rs2Letters))]
	}
	b[8] = '0'

	return b[:]
}

func (sv *Server) dat(w http.ResponseWriter, r *http.Request) { //dat
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

	if bd, ok := sv.boards[bbs]; ok {
		if th, ok := bd.threads[key]; ok {
			w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
			w.Header().Set("Cache-Control", "no-cache") //last-modified等で確認取れない限り再取得
			th.RLock()
			http.ServeContent(w, r, "/"+bbs+"/dat/"+key+".dat", th.lastmod, strings.NewReader(toSJIS(th.dat))) //回数多いためServeContentでキャッシュ保存
			th.RUnlock()

			if sv.Function.WriteChecker != nil {
				if ok := sv.Function.ArchiveChecker(th); ok {
					th.Save(sv.Dir+"/"+bbs+"/kako/", sv.location)
					bd.DeleteThread(key)
					return
				}
			}
		}
	}
}

func (sv *Server) sub(w http.ResponseWriter, r *http.Request) { //subject.txt
	path := r.URL.Path[1:]
	path = strings.TrimSuffix(path, "/")
	bbs := strings.Split(path, "/")[0]
	w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
	w.Header().Set("Cache-Control", "no-cache")
	if bd, ok := sv.boards[bbs]; ok {
		stream_toSJIS(strings.NewReader(bd.subject), w)
	} else {
		dispError(w, "bbsが不正です!")
	}
}

func dispError(w http.ResponseWriter, stat string) {
	w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
	title := "ERROR!"
	body := Escape.Replace(toSJIS(stat))
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

func (sv *Server) NewBoard(bbs, title string) {
	if !exists(sv.Dir + "/" + bbs) {
		os.MkdirAll(sv.Dir+"/"+bbs+"/dat/", 0755)
	}
	bd := &board{}
	bd.init(sv, bbs)
	bd.Config.Raw["BBS_TITLE"] = title
	bd.Config.Raw["BBS_TITLE_ORIG"] = title
	bd.Config.Raw["BBS_NONAME_NAME"] = "名無しさん"
	bd.Config.Raw["BBS_DELETE_NAME"] = "あぼーん"
	bd.Config.Raw["BBS_MAX_RES"] = "1000"
	bd.Config.Raw["BBS_MESSAGE_MAXLEN"] = "2048"
	bd.Config.Raw["BBS_SUBJECT_MAXLEN"] = "30"

	sv.boards[bbs].reloadSettings()
	sv.boards[bbs].saveSettings()
}

func (sv *Server) DeleteBoard(bbs string) error {
	os.RemoveAll(sv.Dir + "/" + bbs)
	if _, ok := sv.boards[bbs]; !ok {
		return errors.New("no such board")
	}
	delete(sv.boards, bbs)
	return nil
}

func (bd *board) DeleteThread(key string) error {
	os.Remove(bd.server.Dir + "/" + bd.bbs + "/dat/" + key + ".dat")
	if _, ok := bd.threads[key]; !ok {
		return errors.New("no such thread")
	}
	delete(bd.threads, key)
	bd.refresh_subjects()
	return nil
}

func (th *Thread) NewRes(res *Res) {
	date_id := strings.Replace(res.Date.Format("2006-01-02(<>) 15:04:05.00"), "<>", wdays[res.Date.Weekday()], 1) + " ID:" + string(res.ID[:]) // 2021-08-25(水) 22:44:30.40 ID:MgUxkbjl0
	outdat := res.From + "<>" + res.Mail + "<>" + date_id + "<>" + res.Message + "<>" + res.Subject + "\n"                                     // 吐き出すDat
	th.Lock()
	th.dat += outdat
	th.num++
	th.lastmod = res.Date
	th.Unlock()
}

func (th *Thread) DeleteRes(num int) error {
	tmp := strings.Split(th.dat, "\n")
	if len(tmp) < num {
		return errors.New("no such res")
	}
	targetres := tmp[num-1]
	tmp = strings.Split(targetres, "<>")
	replaceres := "あぼーん<>" + tmp[1] + "<>" + tmp[2] + "<>あぼーん<>" + tmp[4]
	th.Lock()
	th.dat = strings.Replace(th.dat, targetres, replaceres, 1)
	th.lastmod = time.Now()
	th.Unlock()
	return nil
}

func (abd *adminboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var stat struct {
		Status string      `json:"status,omitempty"`
		Reason string      `json:"reason,omitempty"`
		Data   interface{} `json:"data,omitempty"`
	}
	bbs := Escape.Replace(r.PostFormValue("bbs"))
	key := Escape.Replace(r.PostFormValue("key"))
	boardname := Escape.Replace(r.PostFormValue("boardname"))

	switch {
	case strings.HasSuffix(r.URL.Path, "/newBoard"):
		{
			abd.server.NewBoard(bbs, boardname)
			stat.Status = "Success"
		}
	case strings.HasSuffix(r.URL.Path, "/boardList"):
		{
			type bd struct {
				BBS   string `json:"bbs,omitempty"`
				Title string `json:"title,omitempty"`
			}
			var boards []bd
			for _, v := range abd.server.boards {
				boards = append(boards, bd{
					BBS:   v.bbs,
					Title: v.Config.title,
				})
			}
			stat.Status = "Success"
			stat.Data = boards
		}
	case strings.HasSuffix(r.URL.Path, "/deleteBoard"):
		{
			if err := abd.server.DeleteBoard(bbs); err == nil {
				stat.Status = "Success"
			} else {
				stat.Status = "Failed"
				stat.Reason = err.Error()
			}
		}
	case strings.HasSuffix(r.URL.Path, "/deleteThread"):
		{
			if bd, ok := abd.server.boards[bbs]; ok {
				if err := bd.DeleteThread(key); err == nil {
					stat.Status = "Success"
				} else {
					stat.Status = "Failed"
					stat.Reason = err.Error()
				}
			} else {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
			}
		}
	case strings.HasSuffix(r.URL.Path, "/deleteRes"):
		{
			resnum, err := strconv.Atoi(r.PostFormValue("resnum"))
			if err != nil {
				stat.Status = "Failed"
				stat.Reason = "Invalid resnum"
			} else if bd, ok := abd.server.boards[bbs]; !ok {
				stat.Status = "Failed"
				stat.Reason = "No such board"
			} else if th, ok := bd.threads[key]; !ok {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
			} else {
				if err := th.DeleteRes(resnum); err == nil {
					stat.Status = "Success"
				} else {
					stat.Status = "Failed"
					stat.Reason = err.Error()
				}
			}
		}
	default:
		{
			http.ServeFile(w, r, abd.server.Dir+r.URL.Path)
		}
	}
	if stat.Status != "" {
		json.NewEncoder(w).Encode(stat)
	}
}
