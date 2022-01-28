package gochan

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Dir    string
	Host   string
	boards map[string]*board

	location   *time.Location
	httpserver *http.ServeMux

	Function struct {
		IDGenerator func(string) []byte
		// NGとか
		MessageChecker func(*Res) (bool, string) //res (ok,reason)
	}
}

type board struct {
	bbs     string
	threads map[string]*thread
	Config  struct {
		Raw           map[string]string
		title         string
		threadMaxRes  uint
		messageMaxLen uint
		subjectMaxLen uint
		noName        string
	}
	subject string
	server  *Server
	// Index    *template.Template
}

type thread struct {
	key      string
	lock     sync.RWMutex
	title    string
	dat      string
	num      uint
	firstmod time.Time
	lastmod  time.Time
	board    *board
}

type Res struct {
	From, Mail, Message, Subject string
	ID                           []byte
	Date                         time.Time

	thread *thread
	Req    http.Request
}

func (sv *Server) Init() *Server {
	if sv.Dir == "" {
		sv.Dir = "./Server"
	}
	if sv.Host == "" {
		sv.Host = "0.0.0.0:80"
	}
	sv.Dir = filepath.Clean(sv.Dir)
	sv.boards = map[string]*board{}
	bds := searchboards(sv.Dir)
	if sv.location == nil {
		sv.SetLocation("Asia/Tokyo")
	}

	for _, bbs := range bds { //板情報読み取り
		log.Println("board found: " + bbs)
		bd := &board{}
		bd.init(sv, bbs)
		bd.readSettings()

		keys := searchdats(sv.Dir + "/" + bbs + "/dat")
		for _, key := range keys { //スレ情報読み込み
			th := &thread{}
			th.init(bd, key)
			th.dat = readalltxt(sv.Dir + "/" + bbs + "/dat/" + key + ".dat")
			th.num = uint(strings.Count(th.dat, "\n"))
			tmp := strings.Split(th.dat, "\n")
			th.title = strings.Split(tmp[0], "<>")[4]
			th.key = key
			th.board = bd

			utime, _ := strconv.Atoi(key) //エラーでもどうせ0になるだけなので無視
			th.firstmod = time.Unix(int64(utime), 0)

			info, err := readfileinfo(sv.Dir + "/" + bbs + "/dat/" + key + ".dat")
			if err != nil {
				log.Println(err)
			} else {
				th.lastmod = info.ModTime()
			}
		}
		bd.refresh_subjects()
	}
	if len(bds) == 0 {
		sv.NewBoard("Sample", "サンプル")
		log.Println("No boards Found! Created Sample board")
	}
	sv.httpserver = http.NewServeMux()
	return sv
}

func (sv *Server) SetLocation(loc string) error {
	var err error
	sv.location, err = time.LoadLocation(loc)
	return err
}

func (bd *board) init(sv *Server, bbs string) {
	bd.Config.Raw = map[string]string{}
	bd.server = sv
	bd.bbs = bbs
	bd.threads = map[string]*thread{}
	sv.boards[bbs] = bd
	return
}

func (th *thread) init(bd *board, key string) *thread {
	th.lock = sync.RWMutex{}
	th.key = key
	th.board = bd
	bd.threads[key] = th
	return th
}

func (sv *Server) ListenAndServe() error {
	if sv.httpserver == nil {
		sv.Init()
	}
	sv.httpserver.HandleFunc("/test/bbs.cgi", sv.bbs)
	sv.httpserver.HandleFunc("/admin/", sv.AdminAPI)
	sv.httpserver.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/dat/") { //dat
			sv.dat(w, r)
			return
		} else if strings.Contains(r.URL.Path, "/subject.txt") { //subject.txt
			sv.sub(w, r)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/") || strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
		}
		http.ServeFile(w, r, sv.Dir+r.URL.Path)
	})
	return http.ListenAndServe(sv.Host, sv.httpserver)
}

func searchboards(dir string) []string {
	dir = filepath.Clean(dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	var paths []string
	for _, file := range files {
		if file.IsDir() {
			if exists(filepath.Join(dir, file.Name()) + "/setting.txt") {
				paths = append(paths, file.Name())
				if !exists(filepath.Join(dir, file.Name()) + "/dat/") {
					os.MkdirAll(filepath.Join(dir, file.Name())+"/dat/", 755)
				}
			}
		}
	}
	return paths
}

func searchdats(datdir string) []string {
	datdir = filepath.Clean(datdir)
	files, err := os.ReadDir(datdir)
	if err != nil {
		log.Fatal(err)
	}

	var paths []string
	for _, file := range files {
		if !file.IsDir() {
			filename := file.Name()
			if strings.HasSuffix(filename, ".dat") {
				paths = append(paths, strings.TrimSuffix(filename, ".dat"))
			}
		}
	}
	return paths
}

func exists(name string) bool {
	_, err := os.Stat(name)
	if err != nil {
		return false
	}
	return !os.IsNotExist(err)
}

func readfileinfo(name string) (fs.FileInfo, error) {
	file, err := os.OpenFile(name, os.O_RDONLY, 666)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (sv *Server) Saver() {
	for bbs, b := range sv.boards {
		for key, t := range b.threads {
			path := filepath.Clean(sv.Dir + "/" + bbs + "/dat/" + key + ".dat")
			dat, err := os.Create(path)
			if err != nil {
				log.Println(err)
			}
			dat.WriteString(t.dat)
			dat.Close()

			kakikomis := strings.Split(t.dat, "\n")
			if len(kakikomis)-2 < 0 {
				os.Remove(path)
				continue
			}
			lastkakikomidate := strings.Split(kakikomis[len(kakikomis)-2], "<>")[2] //-2なのは最後が空行で終わるから
			lastkakikomidate = strings.Split(lastkakikomidate, " ID:")[0]
			lastkakikomidate = lastkakikomidate[:strings.Index(lastkakikomidate, "(")] + lastkakikomidate[strings.Index(lastkakikomidate, ")")+1:]
			ti, _ := time.ParseInLocation("2006-01-02 15:04:05.00", lastkakikomidate, sv.location)

			os.Chtimes(path, ti, ti)
		}
	}
}

func (bd *board) saveSettings() {
	path := filepath.Clean(bd.server.Dir + "/" + bd.bbs + "/setting.txt")
	file, err := os.Create(path)
	if err != nil {
		log.Println(err)
		return
	}
	for k, v := range bd.Config.Raw {
		fmt.Fprint(file, toSJIS(k+"="+v+"\r\n"))
	}
}

func (bd *board) readSettings() {
	path := filepath.Clean(bd.server.Dir + "/" + bd.bbs + "/setting.txt")
	txt := readalltxt(path)
	buf := bytes.NewBufferString(toUTF(txt))
	scanner := bufio.NewScanner(buf)

	settings := map[string]string{}
	for scanner.Scan() { //1行ずつ読み出し
		text := scanner.Text()
		strs := strings.SplitN(text, "=", 2)
		if len(strs) > 1 {
			settings[strs[0]] = strs[1] //setting[key] = val
		}
	}
	bd.Config.Raw = settings

	bd.reloadSettings()
}

func (bd *board) reloadSettings() {
	//名無し
	if val, ok := bd.Config.Raw["BBS_NONAME_NAME"]; !ok {
		bd.Config.noName = "名無し"
	} else {
		bd.Config.noName = val
	}

	//タイトル
	if val, ok := bd.Config.Raw["BBS_TITLE"]; !ok {
		if val, ok = bd.Config.Raw["BBS_TITLE_ORIG"]; !ok {
			bd.Config.title = "NoTitle"
		}
	} else {
		bd.Config.title = val
	}

	//スレストまでのレス数
	if val, ok := bd.Config.Raw["BBS_MAX_RES"]; !ok {
		bd.Config.threadMaxRes = 1000
	} else {
		val, err := strconv.Atoi(val)
		if err != nil {
			bd.Config.threadMaxRes = 1000
		} else {
			bd.Config.threadMaxRes = uint(val)
		}
	}

	//レス長さ
	if val, ok := bd.Config.Raw["BBS_MESSAGE_MAXLEN"]; !ok {
		bd.Config.messageMaxLen = 1000
	} else {
		val, err := strconv.Atoi(val)
		if err != nil {
			bd.Config.messageMaxLen = 1000
		} else {
			bd.Config.messageMaxLen = uint(val)
		}
	}

	//スレタイ長さ
	if val, ok := bd.Config.Raw["BBS_SUBJECT_MAXLEN"]; !ok {
		bd.Config.subjectMaxLen = 30
	} else {
		val, err := strconv.Atoi(val)
		if err != nil {
			bd.Config.subjectMaxLen = 30
		} else {
			bd.Config.subjectMaxLen = uint(val)
		}
	}
}

func (bd *board) BBS() string {
	return bd.bbs
}

func (th *thread) Key() string {
	return th.key
}
func (th *thread) Title() string {
	return th.title
}
func (th *thread) Num() uint {
	return th.num
}
func (th *thread) Board() *board {
	return th.board
}

func (rs *Res) Thread() *thread {
	return rs.thread
}
