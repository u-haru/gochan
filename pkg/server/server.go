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
	"strconv"
	"strings"
	"sync"
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
		Raw    map[string]string
		MaxRes uint
		MaxLen uint
		NoName string
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

		sv.Boards[board].Settings.Raw = sv.readsetting(board) //設定

		if _, ok := sv.Boards[board].Settings.Raw["BBS_NONAME_NAME"]; !ok {
			sv.Boards[board].Settings.NoName = "名無し"
		} else {
			sv.Boards[board].Settings.NoName = sv.Boards[board].Settings.Raw["BBS_NONAME_NAME"]
		}

		if _, ok := sv.Boards[board].Settings.Raw["BBS_MAX_RES"]; !ok {
			sv.Boards[board].Settings.MaxRes = 1000
		} else {
			val, _ := strconv.Atoi(sv.Boards[board].Settings.Raw["BBS_MAX_RES"])
			sv.Boards[board].Settings.MaxRes = uint(val)
		}

		if _, ok := sv.Boards[board].Settings.Raw["BBS_MESSAGE_MAXLEN"]; !ok {
			sv.Boards[board].Settings.MaxLen = 1000
		} else {
			val, _ := strconv.Atoi(sv.Boards[board].Settings.Raw["BBS_MESSAGE_MAXLEN"])
			sv.Boards[board].Settings.MaxLen = uint(val)
		}

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
		httpserver.HandleFunc("/", sv.plaintxt)
		httpserver.HandleFunc("/"+i+"/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
			// params := map[string]interface{}{
			// 	"Title": toSJIS(sv.Boards[i].Settings["BBS_TITLE"]),
			// }
			// sv.Boards[i].Index.Execute(w, params)
			http.ServeFile(w, r, sv.Dir+"/"+i+"/index.html")
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
			dat, err := os.Create(filepath.Clean(sv.Dir + "/" + bbs + "/dat/" + key + ".dat"))
			if err != nil {
				log.Println(err)
			}
			dat.WriteString(toSJIS(t.Dat))
			dat.Close()
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

func (sv *Server) readsetting(bbs string) map[string]string {
	path := filepath.Clean(sv.Dir + "/" + bbs + "/setting.txt")
	txt := readalltxt(path)
	buf := bytes.NewBufferString(toUTF(txt))
	scanner := bufio.NewScanner(buf)

	settings := map[string]string{}
	for scanner.Scan() { //1行ずつ読み出し
		text := scanner.Text()
		strs := strings.SplitN(text, "=", 2)
		settings[strs[0]] = strs[1] //setting[key] = val
	}
	return settings
}
