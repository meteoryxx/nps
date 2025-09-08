package controllers

import (
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server"
	"ehang.io/nps/server/tool"
	"github.com/astaxie/beego/logs"

	"github.com/astaxie/beego"
)

type IndexController struct {
	BaseController
}

// getClientOrCreateLocalhost 获取客户端或创建本机客户端
func (s *IndexController) getClientOrCreateLocalhost(clientId int) (*file.Client, error) {
	if clientId == common.LOCALHOST_CLIENT_ID {
		// 创建虚拟的本机客户端
		localClient := &file.Client{
			Id:        common.LOCALHOST_CLIENT_ID,
			Remark:    "本机 (NPS服务器)",
			VerifyKey: "localhost",
			Status:    true,
			IsConnect: true,
			NoStore:   true, // 不存储到文件
			NoDisplay: true, // 不在客户端列表中显示
			Flow:      &file.Flow{},
			Cnf:       &file.Config{},
		}
		return localClient, nil
	}
	return file.GetDb().GetClient(clientId)
}

// Root 处理根路径访问，不返回任何内容
func (s *IndexController) Root() {
	// 不返回任何内容，直接返回空响应
	s.Ctx.Output.SetStatus(404)
	s.Ctx.Output.Body([]byte(""))
}

func (s *IndexController) Index() {
	s.Data["web_base_url"] = beego.AppConfig.String("web_base_url")
	s.Data["data"] = server.GetDashboardData()
	s.SetInfo("dashboard")
	s.display("index/index")
}

func (s *IndexController) Help() {
	s.SetInfo("about")
	s.display("index/help")
}

func (s *IndexController) Tcp() {
	s.SetInfo("tcp")
	s.SetType("tcp")
	s.display("index/list")
}

func (s *IndexController) Udp() {
	s.SetInfo("udp")
	s.SetType("udp")
	s.display("index/list")
}

func (s *IndexController) Socks5() {
	s.SetInfo("socks5")
	s.SetType("socks5")
	s.display("index/list")
}

func (s *IndexController) Http() {
	s.SetInfo("http proxy")
	s.SetType("httpProxy")
	s.display("index/list")
}

func (s *IndexController) File() {
	s.SetInfo("file server")
	s.SetType("file")
	s.display("index/list")
}

func (s *IndexController) Secret() {
	s.SetInfo("secret")
	s.SetType("secret")
	s.display("index/list")
}

func (s *IndexController) P2p() {
	s.SetInfo("p2p")
	s.SetType("p2p")
	s.display("index/list")
}

func (s *IndexController) Host() {
	s.SetInfo("host")
	s.SetType("hostServer")
	s.display("index/list")
}

func (s *IndexController) All() {
	s.Data["menu"] = "client"
	clientId := s.getEscapeString("client_id")
	s.Data["client_id"] = clientId
	s.SetInfo("client id:" + clientId)
	s.display("index/list")
}

func (s *IndexController) GetTunnel() {
	start, length := s.GetAjaxParams()
	taskType := s.getEscapeString("type")
	clientId := s.GetIntNoErr("client_id")
	list, cnt := server.GetTunnel(start, length, taskType, clientId, s.getEscapeString("search"), s.getEscapeString("sort"), s.getEscapeString("order"))
	s.AjaxTable(list, cnt, cnt, nil)
}

func (s *IndexController) Add() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["type"] = s.getEscapeString("type")
		s.Data["client_id"] = s.getEscapeString("client_id")
		s.SetInfo("add tunnel")
		s.display()
	} else {
		clientId := s.GetIntNoErr("client_id")
		id := int(file.GetDb().JsonDb.GetTaskId())

		// 判断是否为本机客户端，如果是则自动设置LocalProxy为true
		localProxy := s.GetBoolNoErr("local_proxy")
		if clientId == common.LOCALHOST_CLIENT_ID {
			localProxy = true
		}

		t := &file.Tunnel{
			Port:                 s.GetIntNoErr("port"),
			ServerIp:             s.getEscapeString("server_ip"),
			Mode:                 s.getEscapeString("type"),
			Target:               &file.Target{TargetStr: s.getEscapeString("target"), LocalProxy: localProxy},
			Id:                   id,
			Status:               true,
			Remark:               s.getEscapeString("remark"),
			Password:             s.getEscapeString("password"),
			LocalPath:            s.getEscapeString("local_path"),
			StripPre:             s.getEscapeString("strip_pre"),
			Flow:                 &file.Flow{},
			BypassGlobalPassword: s.GetBoolNoErr("bypass_global_password"),
		}

		if t.Port <= 0 {
			t.Port = tool.GenerateServerPort(t.Mode)
		}

		if !tool.TestServerPort(t.Port, t.Mode) {
			s.AjaxErr("The port cannot be opened because it may has been occupied or is no longer allowed.")
		}
		var err error
		if t.Client, err = s.getClientOrCreateLocalhost(clientId); err != nil {
			s.AjaxErr(err.Error())
		}
		if t.Client.MaxTunnelNum != 0 && t.Client.GetTunnelNum() >= t.Client.MaxTunnelNum {
			s.AjaxErr("The number of tunnels exceeds the limit")
		}
		if err := file.GetDb().NewTask(t); err != nil {
			s.AjaxErr(err.Error())
		}
		if err := server.AddTask(t); err != nil {
			s.AjaxErr(err.Error())
		} else {
			s.AjaxOkWithId("add success", id)
		}
	}
}

func (s *IndexController) GetOneTunnel() {
	id := s.GetIntNoErr("id")
	data := make(map[string]interface{})
	if t, err := file.GetDb().GetTask(id); err != nil {
		data["code"] = 0
	} else {
		data["code"] = 1
		data["data"] = t
	}
	s.Data["json"] = data
	s.ServeJSON()
}

func (s *IndexController) Edit() {
	id := s.GetIntNoErr("id")
	if s.Ctx.Request.Method == "GET" {
		if t, err := file.GetDb().GetTask(id); err != nil {
			s.error()
		} else {
			s.Data["t"] = t
		}
		s.SetInfo("edit tunnel")
		s.display()
	} else {
		if t, err := file.GetDb().GetTask(id); err != nil {
			s.error()
		} else {
			clientId := s.GetIntNoErr("client_id")
			if client, err := s.getClientOrCreateLocalhost(clientId); err != nil {
				s.AjaxErr("modified error,the client is not exist")
				return
			} else {
				t.Client = client
			}
			if s.GetIntNoErr("port") != t.Port {
				t.Port = s.GetIntNoErr("port")

				if t.Port <= 0 {
					t.Port = tool.GenerateServerPort(t.Mode)
				}

				if !tool.TestServerPort(s.GetIntNoErr("port"), t.Mode) {
					s.AjaxErr("The port cannot be opened because it may has been occupied or is no longer allowed.")
					return
				}
			}
			t.ServerIp = s.getEscapeString("server_ip")
			t.Mode = s.getEscapeString("type")
			t.Target = &file.Target{TargetStr: s.getEscapeString("target")}
			t.Password = s.getEscapeString("password")
			t.Id = id
			t.LocalPath = s.getEscapeString("local_path")
			t.StripPre = s.getEscapeString("strip_pre")
			t.Remark = s.getEscapeString("remark")

			// 判断是否为本机客户端，如果是则自动设置LocalProxy为true
			localProxy := s.GetBoolNoErr("local_proxy")
			if clientId == common.LOCALHOST_CLIENT_ID {
				localProxy = true
			}
			t.Target.LocalProxy = localProxy
			t.BypassGlobalPassword = s.GetBoolNoErr("bypass_global_password")
			file.GetDb().UpdateTask(t)
			server.StopServer(t.Id)
			server.StartTask(t.Id)
		}
		s.AjaxOk("modified success")
	}
}

func (s *IndexController) Stop() {
	id := s.GetIntNoErr("id")
	if err := server.StopServer(id); err != nil {
		s.AjaxErr("stop error")
	}
	s.AjaxOk("stop success")
}

func (s *IndexController) Del() {
	id := s.GetIntNoErr("id")
	if err := server.DelTask(id); err != nil {
		s.AjaxErr("delete error")
	}
	s.AjaxOk("delete success")
}

func (s *IndexController) Start() {
	id := s.GetIntNoErr("id")
	if err := server.StartTask(id); err != nil {
		s.AjaxErr("start error")
	}
	s.AjaxOk("start success")
}

func (s *IndexController) HostList() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["client_id"] = s.getEscapeString("client_id")
		s.Data["menu"] = "host"
		s.SetInfo("host list")
		s.display("index/hlist")
	} else {
		start, length := s.GetAjaxParams()
		clientId := s.GetIntNoErr("client_id")
		list, cnt := file.GetDb().GetHost(start, length, clientId, s.getEscapeString("search"))
		s.AjaxTable(list, cnt, cnt, nil)
	}
}

func (s *IndexController) GetHost() {
	if s.Ctx.Request.Method == "POST" {
		data := make(map[string]interface{})
		if h, err := file.GetDb().GetHostById(s.GetIntNoErr("id")); err != nil {
			data["code"] = 0
		} else {
			data["data"] = h
			data["code"] = 1
		}
		s.Data["json"] = data
		s.ServeJSON()
	}
}

func (s *IndexController) DelHost() {
	id := s.GetIntNoErr("id")
	if err := file.GetDb().DelHost(id); err != nil {
		s.AjaxErr("delete error")
	}
	s.AjaxOk("delete success")
}

func (s *IndexController) AddHost() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["client_id"] = s.getEscapeString("client_id")
		s.Data["menu"] = "host"
		s.SetInfo("add host")
		s.display("index/hadd")
	} else {
		id := int(file.GetDb().JsonDb.GetHostId())
		h := &file.Host{
			Id:                   id,
			Host:                 s.getEscapeString("host"),
			Target:               &file.Target{TargetStr: s.getEscapeString("target"), LocalProxy: s.GetBoolNoErr("local_proxy")},
			HeaderChange:         s.getEscapeString("header"),
			HostChange:           s.getEscapeString("hostchange"),
			Remark:               s.getEscapeString("remark"),
			Location:             s.getEscapeString("location"),
			Flow:                 &file.Flow{},
			Scheme:               s.getEscapeString("scheme"),
			KeyFilePath:          s.getEscapeString("key_file_path"),
			CertFilePath:         s.getEscapeString("cert_file_path"),
			AutoHttps:            s.GetBoolNoErr("AutoHttps"),
			BypassGlobalPassword: s.GetBoolNoErr("bypass_global_password"),
		}
		var err error
		if h.Client, err = file.GetDb().GetClient(s.GetIntNoErr("client_id")); err != nil {
			s.AjaxErr("add error the client can not be found")
		}
		if h.Client.MaxTunnelNum != 0 && h.Client.GetTunnelNum() >= h.Client.MaxTunnelNum {
			s.AjaxErr("The number of tunnels exceeds the limit")
		}

		if err := file.GetDb().NewHost(h); err != nil {
			s.AjaxErr("add fail" + err.Error())
		}
		s.AjaxOkWithId("add success", id)
	}
}

func (s *IndexController) EditHost() {
	id := s.GetIntNoErr("id")
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "host"
		if h, err := file.GetDb().GetHostById(id); err != nil {
			s.error()
		} else {
			s.Data["h"] = h
		}
		s.SetInfo("edit")
		s.display("index/hedit")
	} else {
		if h, err := file.GetDb().GetHostById(id); err != nil {
			s.error()
		} else {
			if h.Host != s.getEscapeString("host") {
				tmpHost := new(file.Host)
				tmpHost.Host = s.getEscapeString("host")
				tmpHost.Location = s.getEscapeString("location")
				tmpHost.Scheme = s.getEscapeString("scheme")
				if file.GetDb().IsHostExist(tmpHost) {
					s.AjaxErr("host has exist")
					return
				}
			}
			if client, err := file.GetDb().GetClient(s.GetIntNoErr("client_id")); err != nil {
				s.AjaxErr("modified error,the client is not exist")
			} else {
				h.Client = client
			}
			h.Host = s.getEscapeString("host")
			h.Target = &file.Target{TargetStr: s.getEscapeString("target")}
			h.HeaderChange = s.getEscapeString("header")
			h.HostChange = s.getEscapeString("hostchange")
			h.Remark = s.getEscapeString("remark")
			h.Location = s.getEscapeString("location")
			h.Scheme = s.getEscapeString("scheme")
			h.KeyFilePath = s.getEscapeString("key_file_path")
			h.CertFilePath = s.getEscapeString("cert_file_path")
			h.Target.LocalProxy = s.GetBoolNoErr("local_proxy")
			h.AutoHttps = s.GetBoolNoErr("AutoHttps")
			h.BypassGlobalPassword = s.GetBoolNoErr("bypass_global_password")
			file.GetDb().JsonDb.StoreHostToJsonFile()
		}
		s.AjaxOk("modified success")
	}
}

func (s *IndexController) ToggleBypassStatus() {
	id := s.GetIntNoErr("id")
	newStatus := s.GetBoolNoErr("status")

	if t, err := file.GetDb().GetTask(id); err != nil {
		logs.Warn("ToggleBypassStatus failed: Task %d not found, error: %v", id, err)
		s.AjaxErr("任务不存在")
		return
	} else {
		t.BypassGlobalPassword = newStatus
		if err := file.GetDb().UpdateTask(t); err != nil {
			logs.Error("ToggleBypassStatus failed: Error updating task %d, error: %v", id, err)
			s.AjaxErr("更新失败: " + err.Error())
			return
		}
		logs.Info("Tunnel %d BypassGlobalPassword status updated to %v", id, newStatus)
		s.AjaxOk("更新成功")
	}
}

// 新增：切换域名记录免验证状态
func (s *IndexController) ToggleHostBypassStatus() {
	id := s.GetIntNoErr("id")
	newStatus := s.GetBoolNoErr("status")

	if h, err := file.GetDb().GetHostById(id); err != nil {
		logs.Warn("ToggleHostBypassStatus failed: Host %d not found, error: %v", id, err)
		s.AjaxErr("域名记录不存在")
		return
	} else {
		h.BypassGlobalPassword = newStatus
		// 保存对 Host 记录的更改
		file.GetDb().JsonDb.StoreHostToJsonFile() // 确保更改被持久化
		logs.Info("Host %d BypassGlobalPassword status updated to %v", id, newStatus)
		s.AjaxOk("更新成功")
	}
}
