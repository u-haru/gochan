package server

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
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

func (sv *Server) bbs(w http.ResponseWriter, r *http.Request) { //bbs.cgiと同じ動きする
	subject := toUTF(r.PostFormValue("subject"))
	from := toUTF(r.PostFormValue("FROM"))
	mail := toUTF(r.PostFormValue("mail"))
	bbs := toUTF(r.PostFormValue("bbs"))
	key := toUTF(r.PostFormValue("key"))
	now := time.Now()
	message := toUTF(r.PostFormValue("MESSAGE"))

	if board, ok := sv.Boards[bbs]; !ok {
		dispError(w, "bbsが不正です!")
		return
	} else {
		if subject != "" { //subjectがあれば新規スレ
			key = fmt.Sprintf("%d", now.Unix())
			if uint(len(subject)) > board.Settings.SubjectMaxLen {
				dispError(w, "タイトルが長すぎます!")
				return
			}
			if _, ok := board.Threads[key]; ok { //すでに同じキーのスレがあったら
				dispError(w, "keyが不正です!")
				return
			}
			if !sv.Config.NoRam {
				board.Threads[key] = &Thread{}
			}
		} else {
			if !sv.Config.NoRam {
				if _, ok := board.Threads[key]; !ok {
					dispError(w, "keyが不正です!")
					return
				}
			} else {
				if !exists(sv.Dir + "/" + bbs + "/dat/" + key + ".dat") {
					dispError(w, "keyが不正です!")
					return
				}
			}
		}
		if from == "" {
			from = board.Settings.NoName
		}
		if uint(len(message)) > board.Settings.MessageMaxLen {
			dispError(w, "本文が長すぎます!")
			return
		}
		if message == "" {
			dispError(w, "本文が空です!")
			return
		}

		message = escape.Replace(message)                                                                                                        //メッセージをエスケープ
		datpath := filepath.Clean(sv.Dir + "/" + bbs + "/dat/" + key + ".dat")                                                                   //datのパス
		date_id := strings.Replace(now.Format("2006-01-02(<>) 15:04:05.00"), "<>", wdays[now.Weekday()], 1) + " " + sv.createid(w, r.RemoteAddr) //2021-08-25(水) 22:44:30.40 ID:MgUxkbjl0
		outdat := from + "<>" + mail + "<>" + date_id + "<>" + message + "<>" + subject + "\n"                                                   //吐き出すDat

		var kakikominum uint
		if sv.Config.NoRam { //RAMにデータを展開していない場合
			dat, err := os.OpenFile(datpath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666) //追記モード
			if err != nil {
				log.Println(err)
			}
			defer dat.Close()
			bytes, _ := ioutil.ReadAll(dat)
			kakikominum = uint(strings.Count(toUTF(string(bytes)), "\n"))
			if kakikominum >= board.Settings.ThreadMaxRes {
				dispError(w, "このスレッドは"+fmt.Sprint(board.Settings.ThreadMaxRes)+"を超えました。\n新しいスレッドを立ててください。")
				return
			} else {
				dat.WriteString(toSJIS(outdat))
				kakikominum++
			}
		} else {
			if board.Threads[key].Num >= board.Settings.ThreadMaxRes {
				dispError(w, "このスレッドは"+fmt.Sprint(board.Settings.ThreadMaxRes)+"を超えました。\n新しいスレッドを立ててください。")
				return
			} else {
				board.Threads[key].Lock.Lock()
				board.Threads[key].Dat += outdat
				board.Threads[key].Num++
				board.Threads[key].Lock.Unlock()
				kakikominum = board.Threads[key].Num
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
		sv.refresh_subjects(bbs, key, subject, fmt.Sprintf("%d", kakikominum))
	}
}

func (sv *Server) refresh_subjects(bbs string, key string, subject string, kakikominum string) {
	// subjects := map[string]string{} //マップ
	var subs []struct {
		key   string
		title string
	}

	var buf *bytes.Buffer
	if sv.Config.NoRam {
		buf = bytes.NewBufferString(toUTF(readalltxt(sv.Dir + "/" + bbs + "/subject.txt")))
	} else {
		buf = bytes.NewBufferString(sv.Boards[bbs].Subject)
	}
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() { //1行ずつ読み出し
		tmp := strings.Split(scanner.Text(), "<>")
		tmpkey := tmp[0][:strings.LastIndex(tmp[0], ".dat")]
		sub := tmp[1]
		// subjects[tmpkey] = sub
		subs = append(subs, struct {
			key   string
			title string
		}{key: tmpkey, title: sub})
	}
	if err := scanner.Err(); err != nil {
		log.Print(err)
	}
	if subject == "" {
		// pos := strings.LastIndex(subjects[key], " (")
		// subject = subjects[key][:pos]
		for _, k := range subs {
			if k.key == key {
				subject = k.title[:strings.LastIndex(k.title, " (")]
			}
		}
	}
	// subjects[key] = subject
	subs = append(subs, struct {
		key   string
		title string
	}{key: key, title: subject})

	var top struct {
		key   string
		title string
	}
	// top := subjects[key] //一番上に持ってくる
	for _, k := range subs {
		if k.key == key {
			top = k
		}
	}

	// tmp := key + ".dat<>" + top + " (" + kakikominum + ")" + "\n"

	// for i, k := range subjects {
	// 	if subjects[i] != top {
	// 		tmp += i + ".dat<>" + k + "\n"
	// 	}
	// }

	tmp := key + ".dat<>" + top.title + " (" + kakikominum + ")" + "\n"

	for _, k := range subs {
		if k.key != top.key {
			tmp += k.key + ".dat<>" + k.title + "\n"
		}
	}

	if sv.Config.NoRam {
		subject_txt, err := os.Create(sv.Dir + "/" + bbs + "/subject.txt") //書き込み
		if err != nil {
			log.Println(err)
		}
		defer subject_txt.Close()
		subject_txt.WriteString(toSJIS(tmp))
	} else {
		sv.Boards[bbs].Subject = tmp
	}
}

func (sv *Server) createid(w http.ResponseWriter, remote string) string {
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

	id := "ID:" + string(b[:])

	return id
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

	if val, ok := sv.Boards[bbs]; ok {
		if val, ok := val.Threads[key]; ok {
			w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
			fmt.Fprint(w, toSJIS(val.Dat))
		}
	}
}

func (sv *Server) sub(w http.ResponseWriter, r *http.Request) { //subject.txt
	path := r.URL.Path[1:]
	path = strings.TrimSuffix(path, "/")
	bbs := strings.Split(path, "/")[0]
	w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
	stream_toSJIS(bytes.NewReader([]byte(sv.Boards[bbs].Subject)), w)
}
func (sv *Server) plaintxt(w http.ResponseWriter, r *http.Request) { //subject.txt
	w.Header().Set("Content-Type", "text/plain; charset=Shift_JIS")
	http.ServeFile(w, r, sv.Dir+r.URL.Path)
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
