package controllers

import (
	"encoding/hex"
	"time"

	"net/url"

	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/crypt"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server/proxy"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

type AuthController struct {
	beego.Controller
}

func (s *AuthController) GetAuthKey() {
	m := make(map[string]interface{})
	defer func() {
		s.Data["json"] = m
		s.ServeJSON()
	}()
	if cryptKey := beego.AppConfig.String("auth_crypt_key"); len(cryptKey) != 16 {
		m["status"] = 0
		return
	} else {
		b, err := crypt.AesEncrypt([]byte(beego.AppConfig.String("auth_key")), []byte(cryptKey))
		if err != nil {
			m["status"] = 0
			return
		}
		m["status"] = 1
		m["crypt_auth_key"] = hex.EncodeToString(b)
		m["crypt_type"] = "aes cbc"
		return
	}
}

func (s *AuthController) GetTime() {
	m := make(map[string]interface{})
	m["time"] = time.Now().Unix()
	s.Data["json"] = m
	s.ServeJSON()
}

// ShowAuthPage displays the global authentication page.
func (s *AuthController) ShowAuthPage() {
	s.Data["return_url"] = s.GetString("return_url", "/") // Default redirect to root if not provided
	s.Data["error"] = s.GetString("error")                // Get potential error message from redirect
	s.TplName = "global_auth.html"                        // Use the new auth template
	// Ensure web_base_url and version are passed to the template if needed by layout/dependencies
	s.Data["web_base_url"] = s.Ctx.Input.GetData("web_base_url")
	s.Data["version"] = s.Ctx.Input.GetData("version")
	s.Render() // Render the template directly, not using display() which assumes layout
}

// HandleAuth handles the submitted password for global authentication.
func (s *AuthController) HandleAuth() {
	password := s.GetString("password")
	returnURL := s.GetString("return_url", "/")
	clientIP := s.Ctx.Input.IP() // Get client IP

	globalConfig := file.GetDb().GetGlobal()

	if globalConfig == nil || globalConfig.GlobalPassword == "" {
		logs.Warn("Global password authentication attempted but not configured.")
		// Not configured, maybe redirect away or show an error?
		// For now, just redirect back to the original URL as if auth is not needed.
		s.Redirect(returnURL, 302)
		return
	}

	if password == globalConfig.GlobalPassword {
		// Password is correct, authenticate the IP
		ipCache := proxy.GetGlobalIpAuthCache() // Get the cache instance
		ip := common.GetIpByAddr(clientIP)      // Ensure we use the IP part only
		ipCache.Authenticate(ip)
		logs.Info("Global password authentication successful for IP: %s. Redirecting to: %s", ip, returnURL)

		// Redirect back to the originally requested URL
		s.Redirect(returnURL, 302)
	} else {
		// Password incorrect, redirect back to auth page with an error message
		logs.Warn("Global password authentication failed for IP: %s", clientIP)
		// Use flash messages or URL parameters to show the error
		// Using URL parameter for simplicity here:
		redirectURL := "/nps_global_auth?error=" + url.QueryEscape("密码错误")
		if returnURL != "" && returnURL != "/" {
			redirectURL += "&return_url=" + url.QueryEscape(returnURL)
		}
		s.Redirect(redirectURL, 302)
	}
}

// GlobalAuth handles both GET and POST requests for the auth page.
func (s *AuthController) GlobalAuth() {
	if s.Ctx.Input.IsPost() {
		s.HandleAuth()
	} else {
		s.ShowAuthPage()
	}
}
