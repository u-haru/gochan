package admin

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/u-haru/gochan"
	"golang.org/x/crypto/scrypt"
)

//go:embed auth.html
var authhtml []byte

//go:embed index.html
var indexhtml []byte

var starttime time.Time

func init() {
	starttime = time.Now()
}

type authkey struct {
	expires time.Time
	str     string
}

type board struct {
	BBS     string `json:"bbs,omitempty"`
	Title   string `json:"title,omitempty"`
	Baseurl string `json:"baseurl,omitempty"`
}

type Board struct {
	Server *gochan.Server
	Path   string
	Hash   string
	keys   []authkey
	sync.Once
	boards []board
}

func Hash(pass string) string {
	salt := []byte("some salt")
	converted, _ := scrypt.Key([]byte(pass), salt, 16384, 8, 1, 32)
	return hex.EncodeToString(converted[:])
}

func (abd *Board) auth(w http.ResponseWriter, r *http.Request) (authorized bool) {
	authorized = false
	if abd.Hash == "noauth" {
		return true
	}
	username := gochan.Escape.Replace(r.PostFormValue("username"))
	password := gochan.Escape.Replace(r.PostFormValue("password"))
	if username == "" || password == "" { //パスがない = クッキー認証
		akey, err := r.Cookie("key")
		if err == nil {
			for i, a := range abd.keys {
				if a.expires.Before(time.Now()) {
					abd.keys = append(abd.keys[:i], abd.keys[i+1:]...)
				}
				if a.str == akey.Value {
					abd.keys[i].expires = time.Now().Add(time.Minute * 10) //10分後に失効
					authorized = true
					return
				}
			}
		}
		return
	} else if abd.Hash == "" { //新規
		abd.Hash = GenPassHash(username, password)
		authorized = true
		return
	} else if !authorized { //パスワード認証
		if abd.Hash == GenPassHash(username, password) { //pass auth
			key := Hash(time.Now().Format("2006-01-02 15:04:05.00"))
			expire := time.Now().Add(time.Minute * 10) //10分後に失効
			http.SetCookie(w, &http.Cookie{
				Name:   "key",
				Value:  key,
				Domain: r.URL.Host,
				Path:   abd.Path,
			})
			abd.keys = append(abd.keys, authkey{str: key, expires: expire})
			authorized = true
			return
		}
	}
	return false
}
func GenPassHash(username, password string) string {
	return Hash(username + password)
}

func (abd *Board) updateboards() {
	abd.boards = []board{}
	for _, v := range abd.Server.Boards() {
		abd.boards = append(abd.boards, board{
			BBS:     v.BBS(),
			Title:   v.Title(),
			Baseurl: v.Server().Baseurl,
		})
	}
	sort.Slice(abd.boards, func(i, j int) bool { return strings.Compare(abd.boards[i].BBS, abd.boards[j].BBS) == -1 })
}
func (abd *Board) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	abd.Do(func() {
		if !strings.HasPrefix(abd.Path, "/") {
			abd.Path = "/" + abd.Path
		}
		if !strings.HasSuffix(abd.Path, "/") {
			abd.Path = abd.Path + "/"
		}
	})
	w.Header().Set("Cache-Control", "no-cache") //last-modified等で確認取れない限り再取得
	if !abd.auth(w, r) {
		http.ServeContent(w, r, fmt.Sprintf("%sauth.html", abd.Path), starttime, bytes.NewReader(authhtml))
		return
	}
	var stat struct {
		Status string      `json:"status,omitempty"`
		Reason string      `json:"reason,omitempty"`
		Data   interface{} `json:"data,omitempty"`
	}
	bbs := gochan.Escape.Replace(r.PostFormValue("bbs"))
	key := gochan.Escape.Replace(r.PostFormValue("key"))
	boardname := gochan.Escape.Replace(r.PostFormValue("boardname"))

	switch {
	case r.URL.Path == abd.Path:
		{
			http.ServeContent(w, r, fmt.Sprintf("%sindex.html", abd.Path), starttime, bytes.NewReader(indexhtml))
		}
	case strings.HasSuffix(r.URL.Path, "/newBoard"):
		{
			if err := abd.Server.NewBoard(bbs, boardname); err == nil {
				abd.updateboards()
				stat.Status = "Success"
				abd.Server.GenBBSmenu()
			} else {
				stat.Status = "Failed"
				stat.Reason = err.Error()
			}
		}
	case strings.HasSuffix(r.URL.Path, "/symLink"):
		{
			if bd, ok := abd.Server.Boards()[bbs]; ok {
				target := path.Join(abd.Server.Path(), "test", "index.html")
				symlink := path.Join(bd.Path(), "index.html")
				if gochan.Escape.Replace(r.PostFormValue("sym")) == "true" {
					//シンボリックリンク作成
					if relpath, err := filepath.Rel(bd.Path(), target); err == nil {
						if err := os.Symlink(relpath, symlink); err != nil {
							stat.Status = "error"
							stat.Reason = err.Error()
						} else {
							stat.Status = "Success"
						}
					}
				} else if gochan.Escape.Replace(r.PostFormValue("sym")) == "false" {
					//シンボリックリンク削除
					if err := os.Remove(symlink); err != nil {
						stat.Status = "error"
						stat.Reason = err.Error()
					} else {
						stat.Status = "Success"
					}
				}
			} else {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
			}
		}
	case strings.HasSuffix(r.URL.Path, "/boardList"):
		{
			if len(abd.boards) == 0 {
				abd.updateboards()
			}
			stat.Status = "Success"
			stat.Data = abd.boards
		}
	case strings.HasSuffix(r.URL.Path, "/deleteBoard"):
		{
			if err := abd.Server.DeleteBoard(bbs); err == nil {
				abd.updateboards()
				stat.Status = "Success"
				abd.Server.GenBBSmenu()
			} else {
				stat.Status = "Failed"
				stat.Reason = err.Error()
			}
		}
	case strings.HasSuffix(r.URL.Path, "/deleteThread"):
		{
			if bd, ok := abd.Server.Boards()[bbs]; ok {
				if err := bd.DeleteThread(key); err == nil {
					stat.Status = "Success"
				} else {
					stat.Status = "Failed"
					stat.Reason = err.Error()
				}
			} else {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
			}
		}
	case strings.HasSuffix(r.URL.Path, "/deleteRes"):
		{
			resnum, err := strconv.Atoi(r.PostFormValue("resnum"))
			if err != nil {
				stat.Status = "Failed"
				stat.Reason = "Invalid resnum"
			} else if bd, ok := abd.Server.Boards()[bbs]; !ok {
				stat.Status = "Failed"
				stat.Reason = "No such board"
			} else if th, ok := bd.Threads()[key]; !ok {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
			} else {
				if err := th.DeleteRes(resnum); err == nil {
					stat.Status = "Success"
				} else {
					stat.Status = "Failed"
					stat.Reason = err.Error()
				}
			}
		}
	case strings.HasSuffix(r.URL.Path, "/configList"):
		{
			if bbs == "" {
				stat.Status = "Success"
				stat.Data = abd.Server.Conf.AllValues()
				break
			}
			bd, ok := abd.Server.Boards()[bbs]
			if !ok {
				stat.Status = "Failed"
				stat.Reason = "No such board"
				break
			}
			if key == "" {
				stat.Status = "Success"
				stat.Data = bd.Conf.AllValues()
				break
			}
			th, ok := bd.Threads()[key]
			if !ok {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
				break
			}
			stat.Status = "Success"
			stat.Data = th.Conf.AllValues()
		}
	case strings.HasSuffix(r.URL.Path, "/setConfig"):
		{
			var conf struct {
				Key   string      `json:"key"`
				Value interface{} `json:"value"`
			}
			err := json.Unmarshal([]byte(r.PostFormValue("json")), &conf)
			if err != nil {
				stat.Status = "Failed"
				stat.Reason = err.Error()
				break
			}
			f, ok := conf.Value.(float64)
			if ok && math.Floor(f) == f {
				conf.Value = int(f)
			}
			log.Println(r.FormValue("json"))
			if bbs == "" {
				if err := abd.Server.Conf.SetWithReflect(conf.Key, conf.Value); err != nil {
					stat.Status = "Failed"
					stat.Reason = err.Error()
					break
				}
				stat.Status = "Success"
				break
			}
			bd, ok := abd.Server.Boards()[bbs]
			if !ok {
				stat.Status = "Failed"
				stat.Reason = "No such board"
				break
			}
			if key == "" {
				if err := bd.Conf.SetWithReflect(conf.Key, conf.Value); err != nil {
					stat.Status = "Failed"
					stat.Reason = err.Error()
					break
				}
				stat.Status = "Success"
				break
			}
			th, ok := bd.Threads()[key]
			if !ok {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
				break
			}
			if err := th.Conf.SetWithReflect(conf.Key, conf.Value); err != nil {
				stat.Status = "Failed"
				stat.Reason = err.Error()
				break
			}
			stat.Status = "Success"
			break
		}
	case strings.HasSuffix(r.URL.Path, "/deleteConfig"):
		{
			var conf struct {
				Key string `json:"key"`
			}
			json.Unmarshal([]byte(r.PostFormValue("json")), &conf)
			if _, ok := gochan.Server_Conf[conf.Key]; ok {
				stat.Status = "Failed"
				stat.Reason = conf.Key + " cant be deleted"
				break
			}
			if bbs == "" {
				stat.Status = "Success"
				abd.Server.Conf.Delete(conf.Key)
				break
			}
			bd, ok := abd.Server.Boards()[bbs]
			if !ok {
				stat.Status = "Failed"
				stat.Reason = "No such board"
				break
			}
			if key == "" {
				stat.Status = "Success"
				bd.Conf.Delete(conf.Key)
				break
			}
			th, ok := bd.Threads()[key]
			if !ok {
				stat.Status = "Failed"
				stat.Reason = "No such thread"
				break
			}
			stat.Status = "Success"
			th.Conf.Delete(conf.Key)
		}
	case strings.HasSuffix(r.URL.Path, "/saveConfig"):
		{
			bd, ok := abd.Server.Boards()[bbs]
			if !ok {
				stat.Status = "Failed"
				stat.Reason = "No such board"
				break
			}
			path := filepath.Clean(bd.Path() + "setting.json")
			file, err := os.Create(path)
			if err != nil {
				log.Println(err)
				return
			}
			defer file.Close()
			if err := bd.Conf.ExportJson(file); err != nil {
				stat.Status = "Failed"
				stat.Reason = err.Error()
				break
			}
			stat.Status = "Success"
		}
	}
	if stat.Status != "" {
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stat)
	}
}
