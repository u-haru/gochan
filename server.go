package gochan

import (
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/u-haru/gochan/pkg/config"
)

type Server struct {
	Dir    string
	Host   string
	boards map[string]*board

	adminboard *adminboard

	location   *time.Location
	httpserver *http.ServeMux

	Conf     config.Config
	Function struct {
		IDGenerator func(string) []byte
		// NGとか
		WriteChecker   func(*Res) (bool, string) //res (ok,reason)
		ArchiveChecker func(*Thread) bool
	}
}

type board struct {
	bbs     string
	title   string
	threads map[string]*Thread
	Conf    config.Config
	subject string
	server  *Server
	// Index    *template.Template
}

type authkey struct {
	expires time.Time
	str     string
}

type adminboard struct {
	foldername string
	server     *Server
	hash       string
	keys       []authkey
}

type Thread struct {
	key      string
	title    string
	dat      string
	num      uint
	firstmod time.Time
	lastmod  time.Time
	board    *board
	sync.RWMutex
	Conf config.Config
}

type Res struct {
	From, Mail, Message, Subject string
	ID                           []byte
	Date                         time.Time

	thread *Thread
	Req    http.Request
	Writer http.ResponseWriter
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
	var bds []*board
	bds, sv.adminboard = sv.searchboards()
	if sv.location == nil {
		sv.SetLocation("Asia/Tokyo")
	}

	for _, bbs := range bds { //板情報読み取り
		log.Println("board found: " + bbs.bbs)
		keys := searchdats(sv.Dir + "/" + bbs.bbs + "/dat")
		for _, key := range keys { //スレ情報読み込み
			th := NewThread(key)
			bbs.AddThread(th)
			th.dat = readalltxt(sv.Dir + "/" + bbs.bbs + "/dat/" + key + ".dat")
			th.num = uint(strings.Count(th.dat, "\n"))
			tmp := strings.SplitN(th.dat, "\n", 2)[0]
			th.title = strings.Split(toUTF(tmp), "<>")[4]
			th.key = key
			th.board = bbs

			utime, _ := strconv.Atoi(key) //エラーでもどうせ0になるだけなので無視
			th.firstmod = time.Unix(int64(utime), 0)

			info, err := readfileinfo(sv.Dir + "/" + bbs.bbs + "/dat/" + key + ".dat")
			if err != nil {
				log.Println(err)
			} else {
				th.lastmod = info.ModTime()
			}
		}
		bbs.refresh_subjects()
	}
	if len(bds) == 0 {
		sv.NewBoard("Sample", "サンプル")
		log.Println("No boards Found! Created Sample board")
	}
	sv.httpserver = http.NewServeMux()

	sv.Conf.Set("NONAME", "名無しさん")
	sv.Conf.Set("DELETED_NAME", "あぼーん")
	sv.Conf.Set("MAX_RES", 1000)
	sv.Conf.Set("MAX_RES_LEN", 2048)
	sv.Conf.Set("SUBJECT_MAXLEN", 30)
	return sv
}

func (sv *Server) SetLocation(loc string) error {
	var err error
	sv.location, err = time.LoadLocation(loc)
	return err
}

func (sv *Server) AddBoard(bd *board) error {
	bd.server = sv
	if bd.bbs == "" {
		return errors.New("board.bbs is empty")
	}
	if _, ok := sv.boards[bd.bbs]; ok {
		return errors.New("bbs already exists")
	}
	bd.Conf.SetParent(&sv.Conf)

	sv.boards[bd.bbs] = bd

	return nil
}

func (sv *Server) NewBoard(bbs, title string) {
	if !exists(sv.Dir + "/" + bbs) {
		os.MkdirAll(sv.Dir+"/"+bbs+"/dat/", 0755)
	}
	bd := NewBoard(bbs)
	sv.AddBoard(bd)
	bd.Conf.Set("BBS_TITLE", title)
	bd.Conf.Set("BBS_TITLE_ORIG", title)

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

// func (bd *board) init(sv *Server, bbs string) {
// 	// bd.Config.Raw = map[string]string{}
// 	bd.server = sv
// 	bd.bbs = bbs
// 	bd.threads = map[string]*Thread{}
// 	bd.Conf.SetParent(&sv.Conf)
// 	sv.boards[bbs] = bd
// }

// func (th *Thread) init(bd *board, key string) *Thread {
// 	th.key = key
// 	th.board = bd
// 	th.Conf.SetParent(&bd.Conf)
// 	bd.threads[key] = th
// 	return th
// }

func (sv *Server) ListenAndServe() error {
	if sv.httpserver == nil {
		sv.Init()
	}
	sv.httpserver.HandleFunc("/test/bbs.cgi", sv.bbs)
	if sv.adminboard != nil {
		sv.httpserver.Handle("/"+sv.adminboard.foldername+"/", sv.adminboard)
	}
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

func (sv *Server) searchboards() ([]*board, *adminboard) {
	dir := filepath.Clean(sv.Dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	var boards []*board
	var admin *adminboard
	for _, file := range files {
		if file.IsDir() {
			if exists(filepath.Join(dir, file.Name()) + "/setting.json") {
				bd := NewBoard(file.Name())
				sv.AddBoard(bd)
				bd.readSettings()
				bd.title, _ = bd.Conf.GetString("TITLE")
				boards = append(boards, bd)
				if !exists(filepath.Join(dir, file.Name()) + "/dat/") {
					os.MkdirAll(filepath.Join(dir, file.Name())+"/dat/", 0755)
				}
			}
			if exists(filepath.Join(dir, file.Name()) + "/adminsetting.txt") {
				admin = &adminboard{
					server:     sv,
					foldername: file.Name(),
					hash:       readalltxt(filepath.Join(dir, file.Name()) + "/adminsetting.txt"),
				}
			}
		}
	}
	return boards, admin
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
	file, err := os.OpenFile(name, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (sv *Server) Save() {
	for bbs, b := range sv.boards {
		for _, t := range b.threads {
			path := sv.Dir + "/" + bbs + "/dat/"
			t.Save(path, sv.location)
		}
	}
}

func (rs *Res) Thread() *Thread {
	return rs.thread
}
