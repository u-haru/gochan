package gochan

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
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
	res.Subject = strings.ReplaceAll(Escape.Replace(toUTF(r.PostFormValue("subject"))), "<br>", "")
	res.From = strings.ReplaceAll(Escape.Replace(toUTF(r.PostFormValue("FROM"))), "<br>", "")
	res.Mail = strings.ReplaceAll(Escape.Replace(toUTF(r.PostFormValue("mail"))), "<br>", "")
	res.Message = Escape.Replace(toUTF(r.PostFormValue("MESSAGE")))
	res.Date = time.Now()
	res.Req = *r
	res.Writer = w

	if board, ok := sv.boards[bbs]; !ok {
		dispError(w, "bbsが不正です!")
		return
	} else {
		if res.Subject != "" { //subjectがあれば新規スレ
			key = fmt.Sprintf("%d", res.Date.Unix())
			th := NewThread(key)
			if err := board.AddThread(th); err != nil {
				dispError(w, "keyが不正です!")
				return
			}
			i, err := board.Conf.GetInt("SUBJECT_MAXLEN")
			if err == nil && len(res.Subject) > i {
				dispError(w, "タイトルが長すぎます!")
				return
			}
			th.title = res.Subject
		}
		th, ok := board.threads[key]
		if !ok {
			dispError(w, "keyが不正です!")
			return
		}
		i, err := th.Conf.GetInt("MAX_RES_LEN")
		if err == nil && len(res.Message) > i {
			dispError(w, "本文が長すぎます!")
			return
		}
		if res.Message == "" {
			dispError(w, "本文が空です!")
			return
		}
		if !th.Writable() {
			dispError(w, "このスレッドは書き込みできる数を超えました。\n新しいスレッドを立ててください。")
			return
		}

		res.thread = th
		if res.From == "" {
			s, err := th.Conf.GetString("NONAME")
			if err == nil {
				res.From = s
			} else {
				res.From = "Noname"
			}
		}

		res.ID = GenerateID(r.RemoteAddr) // ID生成

		if sv.Function.WriteChecker != nil {
			if ok, reason := sv.Function.WriteChecker(res); !ok {
				dispError(w, reason)
				return
			}
		}

		th.AddRes(res)

		if res.Subject != "" { //新規スレの場合にルール生成
			if sv.Function.RuleGenerator != nil {
				sv.Function.RuleGenerator(th)
			}
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

	bd.Lock()
	bd.subject = ""
	for _, k := range subs {
		bd.subject += toSJIS(fmt.Sprintf("%s.dat<>%s (%d)\n", k.key, k.title, k.num))
	}
	bd.lastmod = time.Now()
	bd.Unlock()
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
			http.ServeContent(w, r, "/"+bbs+"/dat/"+key+".dat", th.lastmod, strings.NewReader(th.dat)) //回数多いためServeContentでキャッシュ保存
			th.RUnlock()

			if sv.Function.ArchiveChecker != nil {
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
		bd.RLock()
		http.ServeContent(w, r, "/"+bbs+"/subject.txt", bd.lastmod, strings.NewReader(bd.subject)) //回数多いためServeContentでキャッシュ保存
		bd.RUnlock()
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
	tmp, _ := io.ReadAll(file)
	file.Close()
	return string(tmp)
}

func passhash(pass string) string {
	salt := []byte("some salt")
	converted, _ := scrypt.Key([]byte(pass), salt, 16384, 8, 1, 32)
	return hex.EncodeToString(converted[:])
}

func (abd *adminboard) auth(w http.ResponseWriter, r *http.Request) (authorized bool) {
	authorized = false
	if abd.hash == "noauth" {
		return true
	}
	username := Escape.Replace(r.PostFormValue("username"))
	password := Escape.Replace(r.PostFormValue("password"))
	if username == "" || password == "" { //パスがない = クッキー認証
		akey, err := r.Cookie("key")
		if err == nil {
			for i, a := range abd.keys {
				if a.expires.Before(time.Now()) {
					abd.keys = append(abd.keys[:i], abd.keys[i+1:]...)
				}
				if a.str == akey.Value {
					abd.keys[i].expires = time.Now().Add(time.Minute * 10) //10分後に失効
					authorized = true
					return
				}
			}
		}
		return
	} else if abd.hash == "" { //新規
		file, err := os.OpenFile(fmt.Sprintf("%s/%s/adminsetting.txt", abd.server.Dir, abd.foldername), os.O_RDWR, 0666)
		if err != nil {
			log.Println(err)
			return
		}
		abd.hash = passhash(username + password)
		file.WriteString(abd.hash)
		file.Close()
		authorized = true
		return
	} else if !authorized { //パスワード認証
		if abd.hash == passhash(username+password) { //pass auth
			key := passhash(time.Now().Format("2006-01-02 15:04:05.00"))
			expire := time.Now().Add(time.Minute * 10) //10分後に失効
			http.SetCookie(w, &http.Cookie{
				Name:   "key",
				Value:  key,
				Domain: r.URL.Host,
				Path:   abd.foldername,
			})
			abd.keys = append(abd.keys, authkey{str: key, expires: expire})
			authorized = true
			return
		}
	}
	return false
}

func (abd *adminboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache") //last-modified等で確認取れない限り再取得
	if !abd.auth(w, r) {
		http.ServeFile(w, r, fmt.Sprintf("%s/%s/auth.html", abd.server.Dir, abd.foldername))
		return
	}
	var stat struct {
		Status string      `json:"status,omitempty"`
		Reason string      `json:"reason,omitempty"`
		Data   interface{} `json:"data,omitempty"`
	}
	bbs := Escape.Replace(r.PostFormValue("bbs"))
	key := Escape.Replace(r.PostFormValue("key"))
	boardname := Escape.Replace(r.PostFormValue("boardname"))

	switch {
	case strings.HasSuffix(r.URL.Path, "/adminsetting.txt"):
		{
			return
		}
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
					Title: v.title,
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
