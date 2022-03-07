package gochan

import (
	"errors"
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
	th.board = bd
	if th.key == "" {
		return errors.New("thread.key is empty")
	}
	if _, ok := bd.threads[th.key]; ok {
		return errors.New("key already exists")
	}
	th.Conf.SetParent(&bd.Conf)

	bd.threads[th.key] = th

	return nil
}

func (bd *board) DeleteThread(key string) error {
	os.Remove(bd.server.Dir + "/" + bd.bbs + "/dat/" + key + ".dat")
	if _, ok := bd.threads[key]; !ok {
		return errors.New("no such thread")
	}
	delete(bd.threads, key)
	bd.refresh_subjects()
	return nil
}

func (bd *board) saveSettings() {
	path := filepath.Clean(bd.server.Dir + "/" + bd.bbs + "/setting.json")
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
	path := filepath.Clean(bd.server.Dir + "/" + bd.bbs + "/setting.json")
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

}
func (bd *board) BBS() string {
	return bd.bbs
}
