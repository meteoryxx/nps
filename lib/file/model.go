package file

// global settings
type Glob struct {
	BlackIpList    []string `json:"black_ip_list"`   // 全局黑名单IP列表
	WhiteIpList    []string `json:"white_ip_list"`   // 全局白名单IP列表
	GlobalPassword string   `json:"global_password"` // 全局访问密码
}
