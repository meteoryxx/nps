package routers

import (
	"ehang.io/nps/web/controllers"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/context"
)

func Init() {
	web_base_url := beego.AppConfig.String("web_base_url")
	if len(web_base_url) > 0 {
		ns := beego.NewNamespace(web_base_url,
			beego.NSRouter("/", &controllers.IndexController{}, "*:Index"),
			beego.NSAutoRouter(&controllers.IndexController{}),
			beego.NSAutoRouter(&controllers.LoginController{}),
			beego.NSAutoRouter(&controllers.ClientController{}),
			beego.NSAutoRouter(&controllers.AuthController{}),
			beego.NSAutoRouter(&controllers.GlobalController{}),
			beego.NSCond(func(ctx *context.Context) bool {
				return ctx.Input.Query("token") != ""
			}),
		)
		beego.AddNamespace(ns)
	} else {
		beego.Router("/", &controllers.IndexController{}, "*:Index")
		beego.AutoRouter(&controllers.IndexController{})
		beego.AutoRouter(&controllers.LoginController{})
		beego.AutoRouter(&controllers.ClientController{})
		beego.AutoRouter(&controllers.AuthController{})
		beego.AutoRouter(&controllers.GlobalController{})
		beego.Router("/index/togglebypass", &controllers.IndexController{}, "post:ToggleBypassStatus")         // 添加新路由
		beego.Router("/index/togglehostbypass", &controllers.IndexController{}, "post:ToggleHostBypassStatus") // 添加新路由

		// Global Authentication Route
		beego.Router("/nps_global_auth", &controllers.AuthController{}, "*:GlobalAuth")

	}
}
