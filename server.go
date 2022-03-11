package gochan

import (
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/u-haru/gochan/pkg/config"
)

type Server struct {
	Dir    string
	boards map[string]*board

	Baseurl string

	location     time.Location
	HTTPServeMux http.ServeMux

	Conf     config.Config
	Function struct {
		// NGとか
		WriteChecker   func(*Res) (bool, string) //res (ok,reason)
		ArchiveChecker func(*Thread) bool
		RuleGenerator  func(*Thread)
	}
	BBSMENU
}

type Res struct {
	From, Mail, Message, Subject string
	ID                           []byte
	Date                         time.Time

	thread *Thread
	Req    http.Request
	Writer http.ResponseWriter
}

func NewServer() *Server { return new(Server) }

func (sv *Server) Init(dir string) {
	sv.Dir = filepath.Clean(dir)
	if sv.boards == nil {
		sv.boards = make(map[string]*board)
	}
	if !strings.HasSuffix(sv.Baseurl, "/") {
		sv.Baseurl = sv.Baseurl + "/"
	}
	if !strings.HasSuffix(sv.Dir, "/") {
		sv.Dir = sv.Dir + "/"
	}
	sv.searchboards()

	for bbs, bd := range sv.boards { //板情報読み取り
		log.Println("board found: " + bbs)
		keys := searchdats(bd.Path() + "dat")
		for _, key := range keys { //スレ情報読み込み
			th := NewThread(key)
			bd.AddThread(th)
			th.dat = readalltxt(th.Path())
			th.num = uint(strings.Count(th.dat, "\n"))
			tmp := strings.SplitN(th.dat, "\n", 2)[0]
			th.title = strings.Split(toUTF(tmp), "<>")[4]
			th.key = key
			th.board = bd

			utime, _ := strconv.Atoi(key) //エラーでもどうせ0になるだけなので無視
			th.firstmod = time.Unix(int64(utime), 0)

			info, err := readfileinfo(th.Path())
			if err != nil {
				log.Println(err)
			} else {
				th.lastmod = info.ModTime()
			}

			if sv.Function.RuleGenerator != nil {
				sv.Function.RuleGenerator(th)
			}
		}
		bd.refresh_subjects()
	}

	sv.SetLocation("Asia/Tokyo")

	sv.Conf.Set("NONAME", "名無しさん")
	sv.Conf.Set("DELETED_NAME", "あぼーん")
	sv.Conf.Set("MAX_RES", 1000)
	sv.Conf.Set("MAX_RES_LEN", 2048)
	sv.Conf.Set("SUBJECT_MAXLEN", 30)

	sv.GenBBSmenu()
}

func (sv *Server) SetLocation(loc string) error {
	lo, err := time.LoadLocation(loc)
	if err != nil {
		return err
	}
	sv.location = *lo
	return nil
}

func (sv *Server) AddBoard(bd *board) error {
	bd.server = sv
	if bd.bbs == "" {
		return ErrInvalidBBS
	}
	if _, ok := sv.boards[bd.bbs]; ok {
		return ErrBBSExists
	}
	bd.Conf.SetParent(&sv.Conf)

	sv.boards[bd.bbs] = bd

	return nil
}

func (sv *Server) NewBoard(bbs, title string) {
	if !exists(sv.Dir + "/" + bbs) {
		os.MkdirAll(sv.Dir+bbs+"/dat/", 0755)
	}
	bd := NewBoard(bbs)
	sv.AddBoard(bd)
	bd.Conf.Set("BBS_TITLE", title)
	bd.Conf.Set("BBS_TITLE_ORIG", title)

	sv.boards[bbs].reloadSettings()
	sv.boards[bbs].saveSettings()
}

func (sv *Server) DeleteBoard(bbs string) error {
	os.RemoveAll(sv.Dir + bbs)
	if _, ok := sv.boards[bbs]; !ok {
		return ErrBBSNotExists
	}
	delete(sv.boards, bbs)
	return nil
}

func (sv *Server) ListenAndServe(host string) error {
	if host == "" {
		host = ":http"
	}
	ln, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}
	return sv.Serve(ln)
}

func (sv *Server) Serve(ln net.Listener) error {
	sv.HTTPServeMux.HandleFunc("/test/bbs.cgi", sv.bbs)
	sv.HTTPServeMux.HandleFunc(sv.Baseurl, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/dat/") { //dat
			sv.dat(w, r)
			return
		} else if strings.HasSuffix(r.URL.Path, "/subject.txt") { //subject.txt
			sv.sub(w, r)
			return
		} else if strings.HasSuffix(strings.ToLower(r.URL.Path), "/setting.txt") { //setting.txt
			sv.setting(w, r)
			return
		}
		http.ServeFile(w, r, sv.Dir+strings.TrimPrefix(r.URL.Path, sv.Baseurl))
	})
	sv.HTTPServeMux.HandleFunc(sv.Baseurl+"bbsmenu.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
		http.ServeContent(w, r, sv.Baseurl+"bbsmenu.html", sv.BBSMENU.lastmod, strings.NewReader(sv.BBSMENU.HTML))
	})
	sv.HTTPServeMux.HandleFunc(sv.Baseurl+"bbsmenu.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, sv.Baseurl+"bbsmenu.json", sv.BBSMENU.lastmod, strings.NewReader(sv.BBSMENU.JSON))
	})
	return http.Serve(ln, &sv.HTTPServeMux)
}

func (sv *Server) searchboards() {
	dir := filepath.Clean(sv.Dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		if file.IsDir() {
			if exists(filepath.Join(dir, file.Name()) + "/setting.json") {
				bd := NewBoard(file.Name())
				err := sv.AddBoard(bd)
				if err != nil {
					continue
				}
				bd.readSettings()
				bd.title, _ = bd.Conf.GetString("TITLE")
				if !exists(filepath.Join(dir, file.Name()) + "/dat/") {
					os.MkdirAll(filepath.Join(dir, file.Name())+"/dat/", 0755)
				}
			}
		}
	}
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
			path := sv.Dir + bbs + "/dat/"
			t.Save(path, sv.location)
		}
	}
}
func (sv *Server) Boards() map[string]*board {
	return sv.boards
}

func (rs *Res) Thread() *Thread {
	return rs.thread
}

func (sv *Server) Path() string {
	return sv.Dir
}
