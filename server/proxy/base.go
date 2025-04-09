package proxy

import (
	"errors"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"ehang.io/nps/bridge"
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/conn"
	"ehang.io/nps/lib/file"
	"github.com/astaxie/beego/logs"
)

type Service interface {
	Start() error
	Close() error
}

type NetBridge interface {
	SendLinkInfo(clientId int, link *conn.Link, t *file.Tunnel) (target net.Conn, err error)
}

//BaseServer struct
type BaseServer struct {
	id           int
	bridge       NetBridge
	task         *file.Tunnel
	errorContent []byte
	sync.Mutex
}

// IpAuthCacheEntry holds the authentication timestamp for an IP.
type IpAuthCacheEntry struct {
	AuthenticatedAt time.Time
}

// IpAuthCache stores authenticated IPs with expiration.
type IpAuthCache struct {
	mu              sync.RWMutex
	cache           map[string]IpAuthCacheEntry
	authTTL         time.Duration
	cleanupInterval time.Duration
}

var (
	globalIpAuthCache *IpAuthCache
	once              sync.Once
)

// InitGlobalIpAuthCache initializes the global IP authentication cache.
// ควรเรียกใช้ฟังก์ชันนี้เพียงครั้งเดียวเมื่อเริ่มต้นเซิร์ฟเวอร์
func InitGlobalIpAuthCache(authTTL time.Duration, cleanupInterval time.Duration) {
	once.Do(func() {
		logs.Info("Initializing Global IP Authentication Cache with TTL: %v, Cleanup Interval: %v", authTTL, cleanupInterval)
		globalIpAuthCache = &IpAuthCache{
			cache:           make(map[string]IpAuthCacheEntry),
			authTTL:         authTTL,
			cleanupInterval: cleanupInterval,
		}
		go globalIpAuthCache.startCleanupTimer()
	})
}

// GetGlobalIpAuthCache returns the singleton instance of the IP authentication cache.
func GetGlobalIpAuthCache() *IpAuthCache {
	// Initialize with default values if not already initialized
	if globalIpAuthCache == nil {
		// Default TTL 1 hour, Cleanup every 5 minutes
		InitGlobalIpAuthCache(1*time.Hour, 5*time.Minute)
	}
	return globalIpAuthCache
}

// Authenticate marks an IP as authenticated.
func (c *IpAuthCache) Authenticate(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	logs.Trace("Authenticating IP: %s", ip)
	c.cache[ip] = IpAuthCacheEntry{AuthenticatedAt: time.Now()}
}

// IsAuthenticated checks if an IP is currently authenticated.
func (c *IpAuthCache) IsAuthenticated(ip string) bool {
	c.mu.RLock()
	entry, found := c.cache[ip]
	c.mu.RUnlock() // Unlock ASAP

	if !found {
		logs.Trace("IP %s not found in auth cache", ip)
		return false
	}

	// Check if the authentication has expired
	if time.Since(entry.AuthenticatedAt) > c.authTTL {
		logs.Trace("IP %s authentication expired (Authenticated at: %v, TTL: %v)", ip, entry.AuthenticatedAt, c.authTTL)
		// Optionally remove expired entry here or wait for cleanup
		// c.Remove(ip) // Consider adding a Remove method if immediate removal is needed
		return false
	}
	logs.Trace("IP %s is authenticated (Authenticated at: %v)", ip, entry.AuthenticatedAt)
	return true
}

// cleanup removes expired entries from the cache.
func (c *IpAuthCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	cleanedCount := 0
	now := time.Now()
	for ip, entry := range c.cache {
		if now.Sub(entry.AuthenticatedAt) > c.authTTL {
			delete(c.cache, ip)
			cleanedCount++
		}
	}
	if cleanedCount > 0 {
		logs.Info("Global IP Auth Cache: Cleaned up %d expired entries", cleanedCount)
	}
}

// startCleanupTimer runs the cleanup process periodically.
func (c *IpAuthCache) startCleanupTimer() {
	if c.cleanupInterval <= 0 {
		logs.Warn("Global IP Auth Cache: Cleanup timer interval is zero or negative, cleanup disabled.")
		return
	}
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	logs.Info("Global IP Auth Cache: Cleanup timer started.")
	for range ticker.C {
		logs.Trace("Global IP Auth Cache: Running cleanup task.")
		c.cleanup()
	}
}

// CheckGlobalPasswordAuth checks if global password authentication is required and met.
// It returns true if authentication is required but *not* met, false otherwise.
func CheckGlobalPasswordAuth(remoteAddr string) bool {
	globalConfig := file.GetDb().GetGlobal()
	if globalConfig == nil || globalConfig.GlobalPassword == "" {
		return false // Global password not set, auth not required
	}

	ip := common.GetIpByAddr(remoteAddr)
	ipCache := GetGlobalIpAuthCache() // Ensures cache is initialized

	if ipCache.IsAuthenticated(ip) {
		return false // Already authenticated
	}

	// Authentication is required but not met
	logs.Notice("Global password authentication required but not met for IP: %s", ip)
	return true
}

func NewBaseServer(bridge *bridge.Bridge, task *file.Tunnel) *BaseServer {
	return &BaseServer{
		bridge:       bridge,
		task:         task,
		errorContent: nil,
		Mutex:        sync.Mutex{},
	}
}

//add the flow
func (s *BaseServer) FlowAdd(in, out int64) {
	s.Lock()
	defer s.Unlock()
	s.task.Flow.ExportFlow += out
	s.task.Flow.InletFlow += in
}

//change the flow
func (s *BaseServer) FlowAddHost(host *file.Host, in, out int64) {
	s.Lock()
	defer s.Unlock()
	host.Flow.ExportFlow += out
	host.Flow.InletFlow += in
}

//write fail bytes to the connection
func (s *BaseServer) writeConnFail(c net.Conn) {
	c.Write([]byte(common.ConnectionFailBytes))
	c.Write(s.errorContent)
}

//auth check
func (s *BaseServer) auth(r *http.Request, c *conn.Conn, u, p string) error {
	if u != "" && p != "" && !common.CheckAuth(r, u, p) {
		c.Write([]byte(common.UnauthorizedBytes))
		c.Close()
		return errors.New("401 Unauthorized")
	}
	return nil
}

//check flow limit of the client ,and decrease the allow num of client
func (s *BaseServer) CheckFlowAndConnNum(client *file.Client) error {
	if client.Flow.FlowLimit > 0 && (client.Flow.FlowLimit<<20) < (client.Flow.ExportFlow+client.Flow.InletFlow) {
		return errors.New("Traffic exceeded")
	}
	if !client.GetConn() {
		return errors.New("Connections exceed the current client limit")
	}
	return nil
}

func in(target string, str_array []string) bool {
	sort.Strings(str_array)
	index := sort.SearchStrings(str_array, target)
	if index < len(str_array) && str_array[index] == target {
		return true
	}
	return false
}

//create a new connection and start bytes copying
func (s *BaseServer) DealClient(c *conn.Conn, client *file.Client, addr string,
	rb []byte, tp string, f func(), flow *file.Flow, localProxy bool, task *file.Tunnel) error {

	// 判断访问地址是否在全局黑名单内
	if IsGlobalBlackIp(c.RemoteAddr().String()) {
		c.Close()
		return nil
	}

	// 判断访问地址是否在黑名单内
	if common.IsBlackIp(c.RemoteAddr().String(), client.VerifyKey, client.BlackIpList) {
		c.Close()
		return nil
	}

	link := conn.NewLink(tp, addr, client.Cnf.Crypt, client.Cnf.Compress, c.Conn.RemoteAddr().String(), localProxy)
	if target, err := s.bridge.SendLinkInfo(client.Id, link, s.task); err != nil {
		logs.Warn("get connection from client id %d  error %s", client.Id, err.Error())
		c.Close()
		return err
	} else {
		if f != nil {
			f()
		}
		conn.CopyWaitGroup(target, c.Conn, link.Crypt, link.Compress, client.Rate, flow, true, rb, task)
	}
	return nil
}

// 判断访问地址是否在全局黑名单内
func IsGlobalBlackIp(ipPort string) bool {
	// 判断访问地址是否在全局黑名单内
	global := file.GetDb().GetGlobal()
	if global != nil {
		ip := common.GetIpByAddr(ipPort)
		if in(ip, global.BlackIpList) {
			logs.Error("IP地址[" + ip + "]在全局黑名单列表内")
			return true
		}
	}

	return false
}
