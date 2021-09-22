package server

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
)

func (sv *Server) readSettings(bbs string) {
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
	sv.Boards[bbs].Settings.Raw = settings

	//名無し
	if _, ok := sv.Boards[bbs].Settings.Raw["BBS_NONAME_NAME"]; !ok {
		sv.Boards[bbs].Settings.NoName = "名無し"
	} else {
		sv.Boards[bbs].Settings.NoName = sv.Boards[bbs].Settings.Raw["BBS_NONAME_NAME"]
	}

	//スレストまでのレス数
	if _, ok := sv.Boards[bbs].Settings.Raw["BBS_MAX_RES"]; !ok {
		sv.Boards[bbs].Settings.ThreadMaxRes = 1000
	} else {
		val, _ := strconv.Atoi(sv.Boards[bbs].Settings.Raw["BBS_MAX_RES"])
		sv.Boards[bbs].Settings.ThreadMaxRes = uint(val)
	}

	//レス長さ
	if _, ok := sv.Boards[bbs].Settings.Raw["BBS_MESSAGE_MAXLEN"]; !ok {
		sv.Boards[bbs].Settings.MessageMaxLen = 1000
	} else {
		val, _ := strconv.Atoi(sv.Boards[bbs].Settings.Raw["BBS_MESSAGE_MAXLEN"])
		sv.Boards[bbs].Settings.MessageMaxLen = uint(val)
	}

	//スレタイ長さ
	if _, ok := sv.Boards[bbs].Settings.Raw["BBS_SUBJECT_MAXLEN"]; !ok {
		sv.Boards[bbs].Settings.SubjectMaxLen = 30
	} else {
		val, _ := strconv.Atoi(sv.Boards[bbs].Settings.Raw["BBS_SUBJECT_MAXLEN"])
		sv.Boards[bbs].Settings.SubjectMaxLen = uint(val)
	}
}
