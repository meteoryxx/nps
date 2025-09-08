package controllers

import (
	"strings"

	"ehang.io/nps/lib/file"
)

type GlobalController struct {
	BaseController
}

func (s *GlobalController) Index() {
	//if s.Ctx.Request.Method == "GET" {
	//
	//	return
	//}
	s.Data["menu"] = "global"
	s.SetInfo("global")
	s.display("global/index")

	global := file.GetDb().GetGlobal()
	if global == nil {
		return
	}
	s.Data["globalBlackIpList"] = strings.Join(global.BlackIpList, "\r\n")
	s.Data["globalWhiteIpList"] = strings.Join(global.WhiteIpList, "\r\n")
	s.Data["globalPassword"] = global.GlobalPassword
}

// 添加全局黑名单IP
func (s *GlobalController) Save() {
	//global, err := file.GetDb().GetGlobal()
	//if err != nil {
	//	return
	//}
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "global"
		s.SetInfo("save global")
		s.display()
	} else {

		t := &file.Glob{
			BlackIpList:    RemoveRepeatedElement(strings.Split(s.getEscapeString("globalBlackIpList"), "\r\n")),
			WhiteIpList:    RemoveRepeatedElement(strings.Split(s.getEscapeString("globalWhiteIpList"), "\r\n")),
			GlobalPassword: s.GetString("globalPassword"),
		}

		if err := file.GetDb().SaveGlobal(t); err != nil {
			s.AjaxErr(err.Error())
		}
		s.AjaxOk("save success")
	}
}
