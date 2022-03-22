package gochan

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
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

var written = toSJIS(`<html>
<head>
	<title>書きこみました。</title>
	<meta http-equiv="refresh" content="1;URL=%s/?key=%s">
</head>
<body>
	書きこみが終わりました。<br>
	画面を切り替えるまでしばらくお待ち下さい。
</body>
</html>`)

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

	board, ok := sv.boards[bbs]
	if !ok {
		dispError(w, "bbsが不正です!")
		return
	}
	var th *Thread
	if res.Subject != "" { //subjectがあれば新規スレ
		key = fmt.Sprintf("%d", res.Date.Unix())
		th = NewThread(key)
		th.lastmod = res.Date
		th.Conf.SetParent(&board.Conf) //スレ立てに必要なため仮置
		if _, ok := board.threads[key]; ok {
			dispError(w, "keyが不正です!")
			return
		}
		i, err := board.Conf.GetInt("SUBJECT_MAXLEN")
		if err == nil && len(res.Subject) > i {
			dispError(w, "タイトルが長すぎます!")
			return
		}
		th.title = res.Subject
	} else {
		th, ok = board.threads[key]
		if !ok {
			dispError(w, "keyが不正です!")
			return
		}
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

	res.ID = sv.GenerateID(strings.Split(r.RemoteAddr, ":")[0]) // ID生成

	if sv.Function.WriteChecker != nil {
		if ok, reason := sv.Function.WriteChecker(res); !ok {
			dispError(w, reason)
			return
		}
	}

	th.AddRes(res)

	if res.Subject != "" { //新規スレの場合にルール生成
		if err := board.AddThread(th); err != nil {
			dispError(w, "keyが不正です!")
			return
		}
		if sv.Function.RuleGenerator != nil {
			sv.Function.RuleGenerator(th)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
	fmt.Fprintf(w, written, sv.Baseurl+bbs, key)
	board.RefreshSubjects()
}

var subject = toSJIS("%s.dat<>%s (%d)\n")

// 8バイトのランダムな値+1バイトの"0"を返す
// 日付でIDが変化する
func (sv *Server) GenerateID(str string) []byte {
	str = str + time.Now().In(&sv.location).Format("20060102")
	h := md5.New()
	io.WriteString(h, str) //ip to md5

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

func (sv *Server) dat(w http.ResponseWriter, r *http.Request, bbs, key string) { //dat
	if bd, ok := sv.boards[bbs]; ok {
		if th, ok := bd.threads[key]; ok {
			w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
			w.Header().Set("Cache-Control", "no-cache") //last-modified等で確認取れない限り再取得
			th.RLock()
			http.ServeContent(w, r, th.Path(), th.lastmod, strings.NewReader(th.dat)) //回数多いためServeContentでキャッシュ保存
			th.RUnlock()

			if sv.Function.ArchiveChecker != nil {
				if ok := sv.Function.ArchiveChecker(th, false); ok {
					th.Archive()
				}
			}
			return
		}
		dispError(w, "keyが不正です!")
		return
	}
	dispError(w, "bbsが不正です!")
}

func (sv *Server) sub(w http.ResponseWriter, r *http.Request, bbs string) { //subject.txt
	if bd, ok := sv.boards[bbs]; ok {
		w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
		w.Header().Set("Cache-Control", "no-cache")
		bd.RLock()
		http.ServeContent(w, r, bd.Path()+"subject.txt", bd.lastmod, strings.NewReader(bd.subject)) //回数多いためServeContentでキャッシュ保存
		bd.RUnlock()
	} else {
		dispError(w, "bbsが不正です!")
	}
}

func (sv *Server) setting(w http.ResponseWriter, r *http.Request, bbs string) { //setting.txt
	// w.Header().Set("Cache-Control", "no-cache")//別にキャッシュされても困らない
	if bd, ok := sv.boards[bbs]; ok {
		w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
		bd.RLock()
		http.ServeContent(w, r, bd.Path()+"setting.txt", bd.lastmod, strings.NewReader(bd.setting)) //回数多いためServeContentでキャッシュ保存
		bd.RUnlock()
	} else {
		dispError(w, "bbsが不正です!")
	}
}

func dispError(w http.ResponseWriter, stat string) {
	w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
	w.WriteHeader(400)
	body := Escape.Replace(toSJIS(stat))
	fmt.Fprint(w, `<html>
	<head>
	<title>ERROR!</title>
</head>
	<body>`+body+`</body>
</html>`)
}

func readalltxt(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	tmp, _ := io.ReadAll(file)
	file.Close()
	return string(tmp), nil
}
