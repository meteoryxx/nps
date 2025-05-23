package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"ehang.io/nps/bridge"
	"ehang.io/nps/lib/cache"
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/conn"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/lib/goroutine"
	"ehang.io/nps/server/connection"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

type httpServer struct {
	BaseServer
	httpPort      int
	httpsPort     int
	httpServer    *http.Server
	httpsServer   *http.Server
	httpsListener net.Listener
	useCache      bool
	addOrigin     bool
	cache         *cache.Cache
	cacheLen      int
}

func NewHttp(bridge *bridge.Bridge, c *file.Tunnel, httpPort, httpsPort int, useCache bool, cacheLen int, addOrigin bool) *httpServer {
	httpServer := &httpServer{
		BaseServer: BaseServer{
			task:   c,
			bridge: bridge,
			Mutex:  sync.Mutex{},
		},
		httpPort:  httpPort,
		httpsPort: httpsPort,
		useCache:  useCache,
		cacheLen:  cacheLen,
		addOrigin: addOrigin,
	}
	if useCache {
		httpServer.cache = cache.New(cacheLen)
	}
	return httpServer
}

func (s *httpServer) Start() error {
	var err error
	if s.errorContent, err = common.ReadAllFromFile(filepath.Join(common.GetRunPath(), "web", "static", "page", "error.html")); err != nil {
		s.errorContent = []byte("nps 404")
	}
	if s.httpPort > 0 {
		s.httpServer = s.NewServer(s.httpPort, "http")
		go func() {
			l, err := connection.GetHttpListener()
			if err != nil {
				logs.Error(err)
				os.Exit(0)
			}
			err = s.httpServer.Serve(l)
			if err != nil {
				logs.Error(err)
				os.Exit(0)
			}
		}()
	}
	if s.httpsPort > 0 {
		s.httpsServer = s.NewServer(s.httpsPort, "https")
		go func() {
			s.httpsListener, err = connection.GetHttpsListener()
			if err != nil {
				logs.Error(err)
				os.Exit(0)
			}
			logs.Error(NewHttpsServer(s.httpsListener, s.bridge, s.useCache, s.cacheLen).Start())
		}()
	}
	return nil
}

func (s *httpServer) Close() error {
	if s.httpsListener != nil {
		s.httpsListener.Close()
	}
	if s.httpsServer != nil {
		s.httpsServer.Close()
	}
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	return nil
}

func (s *httpServer) handleTunneling(w http.ResponseWriter, r *http.Request) {
	var host *file.Host
	var err error
	host, err = file.GetDb().GetInfoByHost(r.Host, r)
	if err != nil {
		logs.Debug("the url %s %s %s can't be parsed!", r.URL.Scheme, r.Host, r.RequestURI)
		return
	}

	// 自动 http 301 https
	if host.AutoHttps && r.TLS == nil {
		http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusMovedPermanently)
		return
	}

	// 全局密码认证检查 (在获取 host 之后)
	// 如果 host 设置了 BypassGlobalPassword，则跳过检查
	if host != nil && !host.BypassGlobalPassword && CheckGlobalPasswordAuth(r.RemoteAddr) {
		// 如果需要认证，并且请求的不是验证路径本身
		if !strings.HasPrefix(r.URL.Path, "/nps_global_auth") {
			// 获取 web 管理端口
			webPort, err := beego.AppConfig.Int("web_port")
			if err != nil {
				logs.Error("Failed to get web_port from config:", err)
				http.Error(w, "Server configuration error", http.StatusInternalServerError)
				return
			}

			// 获取协议
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}

			// 获取主机名 (去除端口)
			hostname := r.Host
			if h, _, err := net.SplitHostPort(r.Host); err == nil {
				hostname = h
			}

			// 构造完整的原始 URL
			// 使用 r.RequestURI 通常更直接地反映浏览器地址栏中的路径和查询部分
			originalURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)

			// 构造重定向 URL，将完整的原始 URL 编码后放入 return_url
			redirectURL := fmt.Sprintf("%s://%s:%d/nps_global_auth?return_url=%s", scheme, hostname, webPort, url.QueryEscape(originalURL))

			// 重定向到 web 管理端口
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
		// 如果是验证路径，则继续处理（下面会有专门处理验证请求的逻辑）
	}

	if r.Header.Get("Upgrade") != "" {
		rProxy := NewHttpReverseProxy(s)
		rProxy.ServeHTTP(w, r)
	} else {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
			return
		}
		c, _, err := hijacker.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}

		s.handleHttp(conn.NewConn(c), r)
	}
}

func (s *httpServer) handleHttp(c *conn.Conn, r *http.Request) {
	var (
		host       *file.Host
		target     net.Conn
		err        error
		connClient io.ReadWriteCloser
		scheme     = r.URL.Scheme
		lk         *conn.Link
		targetAddr string
		lenConn    *conn.LenConn
		isReset    bool
		wg         sync.WaitGroup
		remoteAddr string
	)
	defer func() {
		if connClient != nil {
			connClient.Close()
		} else {
			s.writeConnFail(c.Conn)
		}
		c.Close()
	}()
reset:
	if isReset {
		host.Client.AddConn()
	}

	remoteAddr = strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if len(remoteAddr) == 0 {
		remoteAddr = c.RemoteAddr().String()
	}

	if host, err = file.GetDb().GetInfoByHost(r.Host, r); err != nil {
		logs.Notice("the url %s %s %s can't be parsed!, host %s, url %s, remote address %s", r.URL.Scheme, r.Host, r.RequestURI, r.Host, r.URL.Path, remoteAddr)
		// 如果无法解析 host，则默认需要检查全局密码 (如果已设置)
		if CheckGlobalPasswordAuth(c.RemoteAddr().String()) {
			logs.Warn("Global password authentication required (host not found) for HTTP connection from %s, closing.", c.RemoteAddr().String())
			s.writeConnFail(c.Conn)
			c.Close()
			return
		}
		// 如果不需要全局密码，也关闭连接，因为 host 无法解析
		c.Close()
		return
	}

	// 全局密码认证检查 (如果 host 未设置 Bypass)
	if !host.BypassGlobalPassword && CheckGlobalPasswordAuth(c.RemoteAddr().String()) {
		// 对于 handleHttp (非 Upgrade)，理论上已经在 handleTunneling 重定向了。
		// 但如果直接访问 IP:port，可能到这里。此时无法重定向，直接关闭。
		logs.Warn("Global password authentication required for HTTP connection (host: %s) from %s, closing.", host.Host, c.RemoteAddr().String())
		s.writeConnFail(c.Conn)
		c.Close()
		return
	}

	// 判断访问地址是否在全局黑名单内
	if IsGlobalBlackIp(c.RemoteAddr().String()) {
		logs.Warn("IP %s is in global black list, closing connection.", c.RemoteAddr().String())
		c.Close()
		return
	}

	if err := s.CheckFlowAndConnNum(host.Client); err != nil {
		logs.Warn("client id %d, host id %d, error %s, when https connection", host.Client.Id, host.Id, err.Error())
		c.Close()
		return
	}
	if !isReset {
		defer host.Client.AddConn()
	}
	if err = s.auth(r, c, host.Client.Cnf.U, host.Client.Cnf.P); err != nil {
		logs.Warn("auth error", err, r.RemoteAddr)
		return
	}
	if targetAddr, err = host.Target.GetRandomTarget(); err != nil {
		logs.Warn(err.Error())
		return
	}

	lk = conn.NewLink("http", targetAddr, host.Client.Cnf.Crypt, host.Client.Cnf.Compress, r.RemoteAddr, host.Target.LocalProxy)
	if target, err = s.bridge.SendLinkInfo(host.Client.Id, lk, nil); err != nil {
		logs.Notice("connect to target %s error %s", lk.Host, err)
		return
	}
	connClient = conn.GetConn(target, lk.Crypt, lk.Compress, host.Client.Rate, true)

	//read from inc-client
	go func() {
		wg.Add(1)
		isReset = false
		defer connClient.Close()
		defer func() {
			wg.Done()
			if !isReset {
				c.Close()
			}
		}()

		err1 := goroutine.CopyBuffer(c, connClient, host.Client.Flow, nil, "")
		if err1 != nil {
			return
		}

		resp, err := http.ReadResponse(bufio.NewReader(connClient), r)
		if err != nil || resp == nil || r == nil {
			// if there got broken pipe, http.ReadResponse will get a nil
			//break
			return
		} else {
			lenConn := conn.NewLenConn(c)
			if err := resp.Write(lenConn); err != nil {
				logs.Error(err)
				//break
				return
			}
		}
	}()

	for {
		//if the cache start and the request is in the cache list, return the cache
		if s.useCache {
			if v, ok := s.cache.Get(filepath.Join(host.Host, r.URL.Path)); ok {
				n, err := c.Write(v.([]byte))
				if err != nil {
					break
				}
				logs.Trace("%s request, method %s, host %s, url %s, remote address %s, return cache", r.URL.Scheme, r.Method, r.Host, r.URL.Path, c.RemoteAddr().String())
				host.Client.Flow.Add(int64(n), int64(n))
				//if return cache and does not create a new conn with client and Connection is not set or close, close the connection.
				if strings.ToLower(r.Header.Get("Connection")) == "close" || strings.ToLower(r.Header.Get("Connection")) == "" {
					break
				}
				goto readReq
			}
		}

		//change the host and header and set proxy setting
		common.ChangeHostAndHeader(r, host.HostChange, host.HeaderChange, c.Conn.RemoteAddr().String())

		logs.Info("%s request, method %s, host %s, url %s, remote address %s, target %s", r.URL.Scheme, r.Method, r.Host, r.URL.Path, remoteAddr, lk.Host)

		//write
		lenConn = conn.NewLenConn(connClient)
		//lenConn = conn.LenConn
		if err := r.Write(lenConn); err != nil {
			logs.Error(err)
			break
		}
		host.Client.Flow.Add(int64(lenConn.Len), int64(lenConn.Len))

	readReq:
		//read req from connection
		r, err = http.ReadRequest(bufio.NewReader(c))
		if err != nil {
			//break
			return
		}
		r.URL.Scheme = scheme
		//What happened ，Why one character less???
		r.Method = resetReqMethod(r.Method)
		if hostTmp, err := file.GetDb().GetInfoByHost(r.Host, r); err != nil {
			logs.Notice("the url %s %s %s can't be parsed!", r.URL.Scheme, r.Host, r.RequestURI)
			break
		} else if host != hostTmp {
			host = hostTmp
			isReset = true
			connClient.Close()
			goto reset
		}
	}
	wg.Wait()
}

func resetReqMethod(method string) string {
	if method == "ET" {
		return "GET"
	}
	if method == "OST" {
		return "POST"
	}
	return method
}

func (s *httpServer) NewServer(port int, scheme string) *http.Server {
	return &http.Server{
		Addr: ":" + strconv.Itoa(port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = scheme
			s.handleTunneling(w, r)
		}),
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
}

func (s *httpServer) NewServerWithTls(port int, scheme string, l net.Listener, certFile string, keyFile string) error {

	if certFile == "" || keyFile == "" {
		logs.Error("证书文件为空")
		return nil
	}
	var certFileByte = []byte(certFile)
	var keyFileByte = []byte(keyFile)

	config := &tls.Config{}
	config.Certificates = make([]tls.Certificate, 1)

	var err error
	config.Certificates[0], err = tls.X509KeyPair(certFileByte, keyFileByte)
	if err != nil {
		return err
	}

	s2 := &http.Server{
		Addr: ":" + strconv.Itoa(port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = scheme
			s.handleTunneling(w, r)
		}),
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		TLSConfig:    config,
	}

	return s2.ServeTLS(l, "", "")
}
