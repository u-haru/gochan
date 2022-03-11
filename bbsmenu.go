package gochan

import (
	"encoding/json"
	"fmt"
	"time"
)

type bbsmenu_s struct {
	Description      string      `json:"description"`
	MenuList         []menu_list `json:"menu_list"`
	LastModify       int         `json:"last_modify"`
	LastModifyString string      `json:"last_modify_string"`
}

type menu_list struct {
	CategoryName    string             `json:"category_name"`
	CategoryContent []category_content `json:"category_content"`
	CategoryTotal   int                `json:"category_total"`
	CategoryNumber  string             `json:"category_number"`
}

type category_content struct {
	URL           string `json:"url"`
	CategoryName  string `json:"category_name"`
	Category      int    `json:"category"`
	BoardName     string `json:"board_name"`
	CategoryOrder int    `json:"category_order"`
	DirectoryName string `json:"directory_name"`
}

type BBSMENU struct {
	HTML, JSON string
	lastmod    time.Time
}

func (sv *Server) GenBBSmenu() error {
	if sv.boards == nil {
		return ErrBBSNotExists
	}
	var bbsmenu bbsmenu_s
	sv.BBSMENU.lastmod = time.Now()

	bbsmenu.Description = "BBSMENU"
	bbsmenu.LastModify = int(sv.BBSMENU.lastmod.Unix())
	bbsmenu.LastModifyString = sv.BBSMENU.lastmod.Format("2006/01/02(Mon) 15:04:05.00")

	list_map := make(map[string]*menu_list)
	for _, board := range sv.boards {
		var cont category_content
		cont.URL = board.URL()
		cont.CategoryName = "その他"
		cont.BoardName = board.Title()
		cont.DirectoryName = board.BBS()

		if s, err := board.Conf.GetString("CATEGORY"); err == nil {
			cont.CategoryName = s
		}
		if _, ok := list_map[cont.CategoryName]; !ok {
			list_map[cont.CategoryName] = &menu_list{
				CategoryName: cont.CategoryName,
			}
		}
		list_map[cont.CategoryName].CategoryContent = append(list_map[cont.CategoryName].CategoryContent, cont)
	}

	i := 1
	for s, list := range list_map {
		for n, cont := range list.CategoryContent {
			cont.CategoryOrder = n
			cont.Category = i
		}
		list.CategoryName = s
		list.CategoryNumber = fmt.Sprint(i)
		list.CategoryTotal = len(list.CategoryContent)
		bbsmenu.MenuList = append(bbsmenu.MenuList, *list)
	}

	tmp, err := json.Marshal(bbsmenu)
	if err != nil {
		return err
	}
	sv.BBSMENU.JSON = string(tmp)

	sv.BBSMENU.HTML = `<HTML>
	<HEAD>
	<META http-equiv="Content-Type" content="text/html; charset=Shift_JIS">
	<TITLE>` + bbsmenu.Description + `</TITLE>
	</HEAD>
	<BODY>`
	for _, list := range bbsmenu.MenuList {
		sv.BBSMENU.HTML += `<br><br><B>` + list.CategoryName + `</B><br>`
		for _, c := range list.CategoryContent {
			sv.BBSMENU.HTML += `<A HREF=` + c.URL + `>` + c.BoardName + `</A><br>`
		}
	}
	sv.BBSMENU.HTML += `
	<br><br>更新日 ` + sv.BBSMENU.lastmod.Format("2006/01/02") + `</BODY>
	</HTML>`
	sv.BBSMENU.HTML = toSJIS(sv.BBSMENU.HTML)
	return nil
}
