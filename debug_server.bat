@echo off
cd /d %~dp0
echo 正在启动NPS调试模式...
echo 配置路径: %cd%\conf
echo.
go run cmd/nps/nps.go service
pause