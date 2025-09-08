package proxy

import (
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	"ehang.io/nps/bridge"
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/conn"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server/connection"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

type TunnelModeServer struct {
	BaseServer
	process  process
	listener net.Listener
}

// tcp|http|host
func NewTunnelModeServer(process process, bridge NetBridge, task *file.Tunnel) *TunnelModeServer {
	s := new(TunnelModeServer)
	s.bridge = bridge
	s.process = process
	s.task = task
	return s
}

// 开始
func (s *TunnelModeServer) Start() error {
	return conn.NewTcpListenerAndProcess(s.task.ServerIp+":"+strconv.Itoa(s.task.Port), func(c net.Conn) {
		if err := s.CheckFlowAndConnNum(s.task.Client); err != nil {
			logs.Warn("client id %d, task id %d,error %s, when tcp connection", s.task.Client.Id, s.task.Id, err.Error())
			c.Close()
			return
		}
		logs.Trace("new tcp connection,local port %d,client %d,remote address %s", s.task.Port, s.task.Client.Id, c.RemoteAddr())
		s.process(conn.NewConn(c), s)
		s.task.Client.AddConn()
	}, &s.listener)
}

// close
func (s *TunnelModeServer) Close() error {
	return s.listener.Close()
}

// web管理方式
type WebServer struct {
	BaseServer
}

// 开始
func (s *WebServer) Start() error {
	p, _ := beego.AppConfig.Int("web_port")
	if p == 0 {
		stop := make(chan struct{})
		<-stop
	}
	beego.BConfig.WebConfig.Session.SessionOn = true
	beego.SetStaticPath(beego.AppConfig.String("web_base_url")+"/static", filepath.Join(common.GetRunPath(), "web", "static"))
	beego.SetViewsPath(filepath.Join(common.GetRunPath(), "web", "views"))
	err := errors.New("Web management startup failure ")
	var l net.Listener
	if l, err = connection.GetWebManagerListener(); err == nil {
		beego.InitBeforeHTTPRun()
		if beego.AppConfig.String("web_open_ssl") == "true" {
			keyPath := beego.AppConfig.String("web_key_file")
			certPath := beego.AppConfig.String("web_cert_file")
			err = http.ServeTLS(l, beego.BeeApp.Handlers, certPath, keyPath)
		} else {
			err = http.Serve(l, beego.BeeApp.Handlers)
		}
	} else {
		logs.Error(err)
	}
	return err
}

func (s *WebServer) Close() error {
	return nil
}

// new
func NewWebServer(bridge *bridge.Bridge) *WebServer {
	s := new(WebServer)
	s.bridge = bridge
	return s
}

type process func(c *conn.Conn, s *TunnelModeServer) error

// tcp proxy
func ProcessTunnel(c *conn.Conn, s *TunnelModeServer) error {
	// 优先检查访问地址是否在全局白名单内，如果在白名单内则跳过所有验证
	if IsGlobalWhiteIp(c.RemoteAddr().String()) {
		// 白名单内的IP直接通过，不需要任何验证
		targetAddr, err := s.task.Target.GetRandomTarget()
		if err != nil {
			c.Close()
			logs.Warn("tcp port %d ,client id %d,task id %d connect error %s (whitelisted)", s.task.Port, s.task.Client.Id, s.task.Id, err.Error())
			return err
		}
		return s.DealClient(c, s.task.Client, targetAddr, nil, common.CONN_TCP, nil, s.task.Client.Flow, s.task.Target.LocalProxy, s.task)
	}

	// 全局密码认证检查 (如果隧道未设置 Bypass)
	if !s.task.BypassGlobalPassword && CheckGlobalPasswordAuth(c.RemoteAddr().String()) {
		logs.Warn("Global password authentication required for TCP tunnel (TaskID: %d) from %s, closing.", s.task.Id, c.RemoteAddr().String())
		c.Close()
		return errors.New("global password authentication required")
	}

	targetAddr, err := s.task.Target.GetRandomTarget()
	if err != nil {
		c.Close()
		logs.Warn("tcp port %d ,client id %d,task id %d connect error %s", s.task.Port, s.task.Client.Id, s.task.Id, err.Error())
		return err
	}

	return s.DealClient(c, s.task.Client, targetAddr, nil, common.CONN_TCP, nil, s.task.Client.Flow, s.task.Target.LocalProxy, s.task)
}

// http proxy
func ProcessHttp(c *conn.Conn, s *TunnelModeServer) error {
	// 优先检查访问地址是否在全局白名单内，如果在白名单内则跳过所有验证
	if IsGlobalWhiteIp(c.RemoteAddr().String()) {
		_, addr, rb, err, r := c.GetHost()
		if err != nil {
			c.Close()
			logs.Info(err)
			return err
		}
		if r.Method == "CONNECT" {
			c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
			rb = nil
		}
		// 白名单IP直接通过，跳过认证
		return s.DealClient(c, s.task.Client, addr, rb, common.CONN_TCP, nil, s.task.Client.Flow, s.task.Target.LocalProxy, nil)
	}

	// 全局密码认证检查 (如果隧道未设置 Bypass)
	// 注意：HTTP 代理模式下，s.task 可能是总的 HTTP 代理任务，而不是具体的 host
	// 因此，检查全局密码通常应该总是执行，除非整个 HTTP 代理服务被标记为 Bypass (目前模型不支持)
	// 这里我们遵循隧道设置
	if !s.task.BypassGlobalPassword && CheckGlobalPasswordAuth(c.RemoteAddr().String()) {
		logs.Warn("Global password authentication required for HTTP proxy (TaskID: %d) from %s, closing.", s.task.Id, c.RemoteAddr().String())
		c.Close()
		return errors.New("global password authentication required")
	}

	_, addr, rb, err, r := c.GetHost()
	if err != nil {
		c.Close()
		logs.Info(err)
		return err
	}
	if r.Method == "CONNECT" {
		c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		rb = nil
	}
	if err := s.auth(r, c, s.task.Client.Cnf.U, s.task.Client.Cnf.P); err != nil {
		return err
	}
	return s.DealClient(c, s.task.Client, addr, rb, common.CONN_TCP, nil, s.task.Client.Flow, s.task.Target.LocalProxy, nil)
}
