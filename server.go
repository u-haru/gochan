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

var Server_Conf map[string]interface{} = map[string]interface{}{
	"DELETED_NAME":   "あぼーん",
	"NONAME":         "名無しさん",
	"MAX_RES_OVER":   "このスレッドは%dを超えたのでこれ以上書き込めません…<br>次のスレを立ててください。",
	"MAX_RES":        1000,
	"MAX_RES_LEN":    2048,
	"MAX_THREAD":     30,
	"SUBJECT_MAXLEN": 30,

	"TITLE": "Sample", //Board_Conf
}

type Server struct {
	Dir    string
	boards map[string]*board

	Baseurl     string
	Description string

	location time.Location
	http.ServeMux

	Conf     config.Config
	Function struct {
		// NGとか
		WriteChecker   func(*Res) (bool, string) //res (ok,reason)
		ArchiveChecker func(*Thread, bool) bool  // force allows archive without resover
		RuleGenerator  func(*Thread)
	}
	BBSMENU
}

type Res struct {
	From, Mail, Message, Subject string
	RemoteAddr                   net.IP
	ID                           []byte
	Date                         time.Time

	thread *Thread
	Req    http.Request
	Writer http.ResponseWriter
}

func NewServer() *Server { return new(Server) }

func (sv *Server) init(dir string) {
	if sv == nil {
		return
	}
	sv.Dir = filepath.Clean(dir)
	sv.boards = make(map[string]*board)
	if !strings.HasSuffix(sv.Baseurl, "/") {
		sv.Baseurl = sv.Baseurl + "/"
	}
	if !strings.HasSuffix(sv.Dir, "/") {
		sv.Dir = sv.Dir + "/"
	}

	sv.SetLocation("Asia/Tokyo")

	for k, v := range Server_Conf {
		sv.Conf.Set(k, v)
	}

	sv.searchboards()

	for bbs, bd := range sv.boards { //板情報読み取り
		log.Println("board found: " + bbs)
		keys, _ := bd.searchdats()
		for _, key := range keys { //スレ情報読み込み
			th := NewThread(key)
			bd.AddThread(th)
			var err error
			th.dat, err = readalltxt(th.Path())
			if err != nil {
				delete(bd.threads, key)
				continue
			}
			th.num = uint(strings.Count(th.dat, "\n"))
			tmp := strings.SplitN(th.dat, "\n", 2)[0]
			th.title = strings.Split(toUTF(tmp), "<>")[4]
			th.key = key
			th.board = bd

			utime, _ := strconv.Atoi(key) //エラーでもどうせ0になるだけなので無視
			th.firstmod = time.Unix(int64(utime), 0)

			info, err := readfileinfo(th.Path())
			if err == nil {
				th.lastmod = info.ModTime()
			}

			if sv.Function.RuleGenerator != nil {
				sv.Function.RuleGenerator(th)
			}
		}
		bd.RefreshSubjects()
	}
	sv.GenBBSmenu()
}

func (sv *Server) SetLocation(loc string) error {
	if sv == nil {
		return ErrInvalidServer
	}
	lo, err := time.LoadLocation(loc)
	if err != nil {
		return err
	}
	sv.location = *lo
	return nil
}

func (sv *Server) AddBoard(bd *board) error {
	if sv == nil {
		return ErrInvalidServer
	}
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

func (sv *Server) NewBoard(bbs, title string) error {
	if exists(sv.Dir + "/" + bbs) {
		return ErrBBSExists
	}
	if err := os.MkdirAll(sv.Dir+bbs+"/dat/", 0755); err != nil {
		return err
	}
	bd := NewBoard(bbs)
	if err := sv.AddBoard(bd); err != nil {
		return err
	}
	bd.Conf.Set("TITLE", title)
	bd.Conf.SetParent(&sv.Conf)
	bd.title = title

	bd.reloadSettings()
	bd.saveSettings()
	sv.GenBBSmenu()
	return nil
}

func (sv *Server) DeleteBoard(bbs string) error {
	if sv == nil {
		return ErrInvalidServer
	}
	os.RemoveAll(sv.Dir + bbs)
	if _, ok := sv.boards[bbs]; !ok {
		return ErrBBSNotExists
	}
	delete(sv.boards, bbs)
	return nil
}

func (sv *Server) ListenAndServe(host, dir string) error {
	if host == "" {
		host = ":http"
	}
	ln, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}
	return sv.Serve(ln, dir)
}

func (sv *Server) Serve(ln net.Listener, dir string) error {
	sv.init(dir)
	sv.HandleFunc("/test/bbs.cgi", sv.bbs)
	sv.HandleFunc(sv.Baseurl, func(w http.ResponseWriter, r *http.Request) {
		strs := strings.Split(r.URL.Path[1:], "/")
		switch {
		case len(strs) >= 3 && strs[len(strs)-2] == "dat":
			{
				key := strings.TrimSuffix(strs[len(strs)-1], ".dat")
				sv.dat(w, r, strs[len(strs)-3], key)
			}
		case len(strs) >= 2 && strs[len(strs)-1] == "subject.txt":
			sv.sub(w, r, strs[len(strs)-2])
		case len(strs) >= 2 && strings.ToLower(strs[len(strs)-1]) == "setting.txt":
			sv.setting(w, r, strs[len(strs)-2])
		case strs[0] == "bbsmenu.html":
			{
				w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
				http.ServeContent(w, r, sv.Baseurl+"bbsmenu.html", sv.BBSMENU.lastmod, strings.NewReader(sv.BBSMENU.HTML))
			}
		case strs[0] == "bbsmenu.json":
			{
				w.Header().Set("Content-Type", "application/json; charset=UTF-8")
				http.ServeContent(w, r, sv.Baseurl+"bbsmenu.json", sv.BBSMENU.lastmod, strings.NewReader(sv.BBSMENU.JSON))
			}
		default:
			http.ServeFile(w, r, sv.Dir+strings.TrimPrefix(r.URL.Path, sv.Baseurl))
		}
	})
	sv.HandleFunc("/test/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, sv.Dir+r.URL.Path)
	})
	return http.Serve(ln, sv)
}

func (sv *Server) searchboards() {
	if sv == nil {
		return
	}
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

func (bd *board) searchdats() ([]string, error) {
	if bd == nil {
		return nil, ErrBBSNotExists
	}
	files, err := os.ReadDir(bd.Path() + "dat")
	if err != nil {
		return nil, err
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
	return paths, nil
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
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (sv *Server) Save() {
	if sv == nil {
		return
	}
	for bbs, b := range sv.boards {
		for _, t := range b.threads {
			path := sv.Dir + bbs + "/dat/"
			t.Save(path)
		}
	}
}
func (sv *Server) Boards() map[string]*board {
	if sv == nil {
		return nil
	}
	return sv.boards
}

func (rs *Res) Thread() *Thread {
	if rs == nil {
		return nil
	}
	return rs.thread
}

func (sv *Server) Path() string {
	if sv == nil {
		return ""
	}
	return sv.Dir
}
