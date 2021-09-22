package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Dir    string
	Host   string
	Boards map[string]*Board
	Config struct {
		NoRam       bool
		NoWriteDisk bool
	}
}
type Board struct {
	Title    string
	Threads  map[string]*Thread
	Settings struct {
		Raw           map[string]string
		ThreadMaxRes  uint
		MessageMaxLen uint
		SubjectMaxLen uint
		NoName        string
	}
	Subject string
	// Index    *template.Template
}
type Thread struct {
	Lock sync.RWMutex
	Dat  string
	Num  uint
}

func New(dir string) *Server {
	sv := new(Server)
	sv.Dir = filepath.Clean(dir)
	sv.Boards = map[string]*Board{}
	boards := searchboards(sv.Dir)
	for _, board := range boards { //板情報読み取り
		sv.Boards[board] = &Board{Threads: map[string]*Thread{}}
		log.Println("board found: " + board)

		// sv.Boards[board].Index, _ = template.ParseFiles(sv.Dir + "/" + board + "/index.html")

		sv.Boards[board].Subject = sv.getsubjects(board)
		dat, err := os.Create(filepath.Clean(sv.Dir + "/" + board + "/subject.txt")) //とりあえず保存
		if err != nil {
			log.Println(err)
		}
		dat.WriteString(toSJIS(sv.Boards[board].Subject))
		dat.Close()

		sv.readSettings(board) //設定

		if !sv.Config.NoRam {
			keys := searchkeys(sv.Dir + "/" + board + "/dat")
			for _, key := range keys { //スレ情報読み込み
				sv.Boards[board].Threads[key] = &Thread{}
				sv.Boards[board].Threads[key].Lock = sync.RWMutex{}
				sv.Boards[board].Threads[key].Dat = toUTF(readalltxt(sv.Dir + "/" + board + "/dat/" + key + ".dat"))
				sv.Boards[board].Threads[key].Num = uint(strings.Count(sv.Boards[board].Threads[key].Dat, "\n"))
			}
		}
	}
	return sv
}

func (sv *Server) Start() {
	httpserver := http.NewServeMux()
	httpserver.HandleFunc("/test/bbs.cgi", sv.bbs)
	for i := range sv.Boards {
		httpserver.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/") {
				w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
			}
			http.ServeFile(w, r, sv.Dir+r.URL.Path)
		})
		if !sv.Config.NoRam {
			httpserver.HandleFunc("/"+i+"/dat/", sv.dat)
			httpserver.HandleFunc("/"+i+"/subject.txt", sv.sub)
		} else {
			httpserver.HandleFunc("/"+i+"/dat/", sv.plaintxt)
			httpserver.HandleFunc("/"+i+"/subject.txt", sv.plaintxt)
		}
	}
	log.Println("Listening on: " + sv.Host)
	log.Println(http.ListenAndServe(sv.Host, httpserver))
}

func searchboards(dir string) []string {
	dir = filepath.Clean(dir)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
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

func searchkeys(datdir string) []string {
	datdir = filepath.Clean(datdir)
	files, err := ioutil.ReadDir(datdir)
	if err != nil {
		panic(err)
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

func (sv *Server) Saver() {
	for bbs, b := range sv.Boards {
		for key, t := range b.Threads {
			path := filepath.Clean(sv.Dir + "/" + bbs + "/dat/" + key + ".dat")
			dat, err := os.Create(path)
			if err != nil {
				log.Println(err)
			}
			dat.WriteString(toSJIS(t.Dat))
			dat.Close()

			kakikomis := strings.Split(t.Dat, "\n")
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

func (sv *Server) getsubjects(bbs string) string {
	datpath := filepath.Clean(sv.Dir + "/" + bbs + "/dat/")
	files, err := ioutil.ReadDir(datpath)
	if err != nil {
		panic(err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) }) //日付順
	subjects := ""
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".dat") {
			dat := readalltxt(sv.Dir + "/" + bbs + "/dat/" + file.Name())
			buf := bytes.NewBufferString(toUTF(dat))
			scanner := bufio.NewScanner(buf)
			scanner.Scan()

			num := uint(strings.Count(toUTF(string(dat)), "\n"))

			subject := strings.Split(scanner.Text(), "<>")[4]
			subjects += file.Name() + "<>" + subject + " (" + fmt.Sprintf("%d", num) + ")\n"
		}
	}
	return subjects
}
