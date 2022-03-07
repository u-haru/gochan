package gochan

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/u-haru/gochan/pkg/config"
)

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

func NewThread(key string) *Thread {
	th := &Thread{}
	th.key = key
	return th
}

func (th *Thread) AddRes(res *Res) {
	date_id := strings.Replace(res.Date.Format("2006-01-02(<>) 15:04:05.00"), "<>", wdays[res.Date.Weekday()], 1) + " ID:" + string(res.ID[:]) // 2021-08-25(水) 22:44:30.40 ID:MgUxkbjl0
	outdat := res.From + "<>" + res.Mail + "<>" + date_id + "<>" + res.Message + "<>" + res.Subject + "\n"                                     // 吐き出すDat
	th.Lock()
	th.dat += toSJIS(outdat)
	th.num++
	th.lastmod = res.Date
	th.Unlock()
}

func (th *Thread) DeleteRes(num int) error {
	tmp := strings.SplitN(th.dat, "\n", num)
	if len(tmp) < num {
		return errors.New("no such res")
	}
	targetres := tmp[num-1]
	tmp = strings.Split(targetres, "<>")
	replaceres := toSJIS("あぼーん<>" + tmp[1] + "<>" + tmp[2] + "<>あぼーん<>" + tmp[4])
	th.Lock()
	th.dat = strings.Replace(th.dat, targetres, replaceres, 1)
	th.lastmod = time.Now()
	th.Unlock()
	return nil
}

// From,Mail,Message,Subject only
func (th *Thread) GetRes(num int) (*Res, error) {
	tmp := strings.SplitN(th.dat, "\n", num)
	if len(tmp) < num {
		return nil, errors.New("no such res")
	}
	targetres := toUTF(tmp[num-1])
	tmp = strings.Split(targetres, "<>")

	res := &Res{
		From:    tmp[0],
		Mail:    tmp[1],
		Message: tmp[3],
		Subject: tmp[4],
	}
	return res, nil
}

func (th *Thread) Save(dir string, location *time.Location) {
	os.MkdirAll(dir, 0755)
	path := filepath.Clean(dir + "/" + th.key + ".dat")
	dat, err := os.Create(path)
	if err != nil {
		log.Println(err)
	}
	dat.WriteString(th.dat)
	dat.Close()

	kakikomis := strings.Split(th.dat, "\n")
	if len(kakikomis)-2 < 0 {
		os.Remove(path)
		return
	}
	lastkakikomidate := strings.Split(kakikomis[len(kakikomis)-2], "<>")[2] //-2なのは最後が空行で終わるから
	lastkakikomidate = strings.Split(lastkakikomidate, " ID:")[0]
	lastkakikomidate = lastkakikomidate[:strings.Index(lastkakikomidate, "(")] + lastkakikomidate[strings.Index(lastkakikomidate, ")")+1:]
	ti, _ := time.ParseInLocation("2006-01-02 15:04:05.00", lastkakikomidate, location)

	os.Chtimes(path, ti, ti)
}

func (th *Thread) Writable() bool {
	i, err := th.Conf.GetInt("MAX_RES")
	if err != nil {
		return false
	}
	return th.num < uint(i)
}

func (th *Thread) Key() string {
	return th.key
}
func (th *Thread) Title() string {
	return th.title
}
func (th *Thread) Num() uint {
	return th.num
}
func (th *Thread) Board() *board {
	return th.board
}
func (th *Thread) Lastmod() time.Time {
	return th.lastmod
}
