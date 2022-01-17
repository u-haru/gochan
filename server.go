package gochan

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type server struct {
	Dir    string
	Host   string
	Boards map[string]*board
	Config struct {
	}
	httpserver *http.ServeMux

	Function struct {
		IDGenerator func(string) [9]byte
		// NGとか
		MessageChecker func(*Res) (bool, string) //res (ok,reason)
	}
}

type board struct {
	bbs     string
	Threads map[string]*thread
	Config  struct {
		Raw           map[string]string
		threadMaxRes  uint
		messageMaxLen uint
		subjectMaxLen uint
		noName        string
	}
	Subject string
	server  *server
	// Index    *template.Template
}

type thread struct {
	lock    sync.RWMutex
	Title   string
	dat     string
	num     uint
	lastmod time.Time
	board   *board
}

type Res struct {
	From, Mail, Message, Subject string
	ID                           [9]byte
	Date                         time.Time
}

func NewServer(dir string) *server {
	sv := new(server)
	sv.Dir = filepath.Clean(dir)
	sv.Boards = map[string]*board{}
	bds := searchboards(sv.Dir)
	for _, bbs := range bds { //板情報読み取り
		log.Println("board found: " + bbs)
		sv.InitBoard(bbs)
		sv.Boards[bbs].readSettings()

		keys := searchdats(sv.Dir + "/" + bbs + "/dat")
		for _, key := range keys { //スレ情報読み込み
			sv.Boards[bbs].NewThread(key)
			sv.Boards[bbs].Threads[key].dat = toUTF(readalltxt(sv.Dir + "/" + bbs + "/dat/" + key + ".dat"))
			sv.Boards[bbs].Threads[key].num = uint(strings.Count(sv.Boards[bbs].Threads[key].dat, "\n"))
			tmp := strings.Split(sv.Boards[bbs].Threads[key].dat, "\n")
			sv.Boards[bbs].Threads[key].Title = strings.Split(tmp[0], "<>")[4]

			lastkakikomidate := strings.Split(tmp[len(tmp)-2], "<>")[2] //-2なのは最後が空行で終わるから
			lastkakikomidate = strings.Split(lastkakikomidate, " ID:")[0]
			lastkakikomidate = lastkakikomidate[:strings.Index(lastkakikomidate, "(")] + lastkakikomidate[strings.Index(lastkakikomidate, ")")+1:]
			t, err := time.Parse("2006-01-02 15:04:05.00", lastkakikomidate)
			if err != nil {
				log.Println(err)
			} else {
				sv.Boards[bbs].Threads[key].lastmod = t
			}
		}
	}
	if len(bds) == 0 {
		sv.NewBoard("Sample", "サンプル")
		log.Println("No boards Found! Created Sample board")
	}
	sv.httpserver = http.NewServeMux()
	return sv
}

func (sv *server) InitBoard(bbs string) *board {
	bd := &board{Threads: map[string]*thread{}}
	bd.Config.Raw = map[string]string{}
	bd.server = sv
	bd.bbs = bbs
	bd.loadsubject()
	sv.Boards[bbs] = bd
	return bd
}

func (sv *server) ListenAndServe() error {
	sv.httpserver.HandleFunc("/test/bbs.cgi", sv.bbs)
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

func (sv *server) Saver() {
	for bbs, b := range sv.Boards {
		for key, t := range b.Threads {
			path := filepath.Clean(sv.Dir + "/" + bbs + "/dat/" + key + ".dat")
			dat, err := os.Create(path)
			if err != nil {
				log.Println(err)
			}
			dat.WriteString(toSJIS(t.dat))
			dat.Close()

			kakikomis := strings.Split(t.dat, "\n")
			if len(kakikomis)-2 < 0 {
				os.Remove(path)
				continue
			}
			lastkakikomidate := strings.Split(kakikomis[len(kakikomis)-2], "<>")[2] //-2なのは最後が空行で終わるから
			lastkakikomidate = strings.Split(lastkakikomidate, " ID:")[0]
			lastkakikomidate = lastkakikomidate[:strings.Index(lastkakikomidate, "(")] + lastkakikomidate[strings.Index(lastkakikomidate, ")")+1:]
			ti, _ := time.Parse("2006-01-02 15:04:05.00", lastkakikomidate)

			os.Chtimes(path, ti, ti)
		}
		dat, err := os.Create(filepath.Clean(sv.Dir + "/" + bbs + "/subject.txt"))
		if err != nil {
			log.Println(err)
		}
		dat.WriteString(toSJIS(b.Subject))
		dat.Close()
	}
}

func (bd *board) loadsubject() {
	datpath := bd.server.Dir + "/" + bd.bbs + "/dat/"
	files, err := os.ReadDir(filepath.Clean(datpath))
	if err != nil {
		log.Fatal(err)
	}

	sort.Slice(files, func(i, j int) bool {
		info_i, _ := files[i].Info()
		info_j, _ := files[j].Info()
		return info_i.ModTime().After(info_j.ModTime())
	}) //日付順
	subjects := ""
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".dat") {
			dat := readalltxt(datpath + file.Name())
			buf := bytes.NewBufferString(toUTF(dat))
			scanner := bufio.NewScanner(buf)
			scanner.Scan()

			num := uint(strings.Count(toUTF(string(dat)), "\n"))

			tmp := strings.Split(scanner.Text(), "<>")
			if len(tmp) < 4 {
				os.Remove(datpath + file.Name())
				continue
			}
			subjects += file.Name() + "<>" + tmp[4] + " (" + fmt.Sprintf("%d", num) + ")\n"
		}
	}
	bd.Subject = subjects
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
