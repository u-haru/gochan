package gochan

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/u-haru/gochan/pkg/config"
)

type board struct {
	bbs     string
	title   string
	threads map[string]*Thread
	Conf    config.Config
	subject string
	setting string
	server  *Server
	lastmod time.Time
	sync.RWMutex
	// Index    *template.Template
}

func NewBoard(bbs string) *board {
	bd := &board{}
	bd.bbs = bbs
	bd.threads = map[string]*Thread{}
	return bd
}

func (bd *board) AddThread(th *Thread) error {
	if bd == nil {
		return ErrBBSNotExists
	}
	th.board = bd
	if th.key == "" {
		return ErrInvalidKey
	}
	if _, ok := bd.threads[th.key]; ok {
		return ErrKeyExists
	}
	th.Conf.SetParent(&bd.Conf)

	bd.threads[th.key] = th

	return nil
}

func (bd *board) DeleteThread(key string) error {
	if bd == nil {
		return ErrBBSNotExists
	}
	if th, ok := bd.threads[key]; !ok {
		return ErrKeyNotExists
	} else {
		delete(bd.threads, key)
		os.Remove(th.Path())
	}
	bd.refresh_subjects()
	return nil
}

func (bd *board) saveSettings() {
	if bd == nil {
		return
	}
	path := filepath.Clean(bd.Path() + "setting.json")
	file, err := os.Create(path)
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()
	bd.Conf.ExportJson(file)
	// for k, v := range bd.Config.Raw {
	// 	fmt.Fprint(file, toSJIS(k+"="+v+"\r\n"))
	// }
}

func (bd *board) readSettings() {
	if bd == nil {
		return
	}
	path := filepath.Clean(bd.Path() + "setting.json")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	bd.Conf.LoadJson(f)
	bd.Conf.SetParent(&bd.server.Conf)

	bd.reloadSettings()
}

func (bd *board) reloadSettings() {
	if bd == nil {
		return
	}
	bd.Lock()
	title, _ := bd.Conf.GetString("TITLE")
	title = toSJIS(title)
	noname, _ := bd.Conf.GetString("NONAME")
	noname = toSJIS(noname)
	bd.setting = fmt.Sprintf("BBS_TITLE=%s\nBBS_TITLE_ORIG=%s\nBBS_NONAME_NAME=%s\n", title, title, noname)
	bd.Unlock()
}

func (bd *board) Threads() map[string]*Thread {
	if bd == nil {
		return nil
	}
	return bd.threads
}

func (bd *board) BBS() string {
	if bd == nil {
		return ""
	}
	return bd.bbs
}

func (bd *board) Title() string {
	if bd == nil {
		return ""
	}
	return bd.title
}

func (bd *board) Server() *Server {
	if bd == nil {
		return nil
	}
	return bd.server
}

func (bd *board) URL() string {
	if bd == nil {
		return ""
	}
	return bd.server.Baseurl + bd.bbs + "/"
}

func (bd *board) Path() string {
	if bd == nil {
		return ""
	}
	return bd.server.Dir + bd.bbs + "/"
}
