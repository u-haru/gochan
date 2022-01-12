package gochan

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
		NoRam bool
	}
	httpserver *http.ServeMux
}

type board struct {
	Title   string
	Threads map[string]*thread
	Config  struct {
		Raw           map[string]string
		ThreadMaxRes  uint
		MessageMaxLen uint
		SubjectMaxLen uint
		NoName        string
	}
	Subject string
	// Index    *template.Template
}

type thread struct {
	Lock sync.RWMutex
	Dat  string
	Num  uint
}

func New(dir string) *server {
	sv := new(server)
	sv.Dir = filepath.Clean(dir)
	sv.Boards = map[string]*board{}
	bds := searchboards(sv.Dir)
	for _, bd := range bds { //板情報読み取り
		sv.Boards[bd] = &board{Threads: map[string]*thread{}}
		log.Println("board found: " + bd)

		// sv.Boards[board].Index, _ = template.ParseFiles(sv.Dir + "/" + board + "/index.html")

		sv.Boards[bd].Subject = sv.getsubjects(bd)
		dat, err := os.Create(filepath.Clean(sv.Dir + "/" + bd + "/subject.txt")) //とりあえず保存
		if err != nil {
			log.Println(err)
		}
		dat.WriteString(toSJIS(sv.Boards[bd].Subject))
		dat.Close()

		sv.readSettings(bd) //設定

		if !sv.Config.NoRam {
			keys := searchkeys(sv.Dir + "/" + bd + "/dat")
			for _, key := range keys { //スレ情報読み込み
				sv.Boards[bd].Threads[key] = &thread{}
				sv.Boards[bd].Threads[key].Lock = sync.RWMutex{}
				sv.Boards[bd].Threads[key].Dat = toUTF(readalltxt(sv.Dir + "/" + bd + "/dat/" + key + ".dat"))
				sv.Boards[bd].Threads[key].Num = uint(strings.Count(sv.Boards[bd].Threads[key].Dat, "\n"))
			}
		}
	}
	sv.httpserver = http.NewServeMux()
	return sv
}
func (sv *server) NewBoard(bbs string) {
	sv.Boards[bbs] = &board{Threads: map[string]*thread{}}
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

func (sv *server) Saver() {
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

func (sv *server) getsubjects(bbs string) string {
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

func (sv *server) readSettings(bbs string) {
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
	sv.Boards[bbs].Config.Raw = settings

	//名無し
	if val, ok := sv.Boards[bbs].Config.Raw["BBS_NONAME_NAME"]; !ok {
		sv.Boards[bbs].Config.NoName = "名無し"
	} else {
		sv.Boards[bbs].Config.NoName = val
	}

	//スレストまでのレス数
	if val, ok := sv.Boards[bbs].Config.Raw["BBS_MAX_RES"]; !ok {
		sv.Boards[bbs].Config.ThreadMaxRes = 1000
	} else {
		val, err := strconv.Atoi(val)
		if err != nil {
			sv.Boards[bbs].Config.ThreadMaxRes = 1000
		} else {
			sv.Boards[bbs].Config.ThreadMaxRes = uint(val)
		}
	}

	//レス長さ
	if val, ok := sv.Boards[bbs].Config.Raw["BBS_MESSAGE_MAXLEN"]; !ok {
		sv.Boards[bbs].Config.MessageMaxLen = 1000
	} else {
		val, err := strconv.Atoi(val)
		if err != nil {
			sv.Boards[bbs].Config.MessageMaxLen = 1000
		} else {
			sv.Boards[bbs].Config.MessageMaxLen = uint(val)
		}
	}

	//スレタイ長さ
	if val, ok := sv.Boards[bbs].Config.Raw["BBS_SUBJECT_MAXLEN"]; !ok {
		sv.Boards[bbs].Config.SubjectMaxLen = 30
	} else {
		val, err := strconv.Atoi(val)
		if err != nil {
			sv.Boards[bbs].Config.SubjectMaxLen = 30
		} else {
			sv.Boards[bbs].Config.SubjectMaxLen = uint(val)
		}
	}
}
