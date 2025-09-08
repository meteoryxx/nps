@echo off
cd /d %~dp0
echo ================================
echo      NPS 调试信息收集
echo ================================
echo.

echo [系统信息]
echo 操作系统: %OS%
echo 架构: %PROCESSOR_ARCHITECTURE%
echo Go版本:
go version
echo.

echo [项目结构]
echo 当前目录: %cd%
echo 配置目录: %cd%\conf
echo.

echo [配置文件检查]
if exist "conf\nps.conf" (
    echo ✓ nps.conf 存在
) else (
    echo ✗ nps.conf 不存在
)

if exist "conf\global.json" (
    echo ✓ global.json 存在
    echo 内容:
    type conf\global.json
) else (
    echo ✗ global.json 不存在
)
echo.

echo [Go模块信息]
go mod tidy
go list -m all | findstr "ehang.io/nps"
echo.

echo [端口检查]
echo 检查端口占用情况:
netstat -an | findstr ":8024"
netstat -an | findstr ":8081"
netstat -an | findstr ":88"
echo.

echo [编译测试]
echo 测试编译...
go build -o test_nps.exe cmd/nps/nps.go
if exist test_nps.exe (
    echo ✓ 编译成功
    del test_nps.exe
) else (
    echo ✗ 编译失败
)
echo.

echo ================================
echo 调试信息收集完成
echo ================================
pause