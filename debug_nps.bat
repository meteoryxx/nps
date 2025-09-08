@echo off
cd /d %~dp0
echo ================================
echo        NPS 调试工具
echo ================================
echo.
echo 请选择调试方式:
echo 1. 直接运行 (查看输出日志)
echo 2. Delve调试器 (断点调试)
echo 3. 构建并运行 (生产模式)
echo 4. 测试白名单功能
echo.
set /p choice="请输入选择(1-4): "

if "%choice%"=="1" (
    echo.
    echo 启动NPS服务 - 直接运行模式...
    echo 提示: 按 Ctrl+C 停止服务
    echo.
    go run cmd/nps/nps.go service
) else if "%choice%"=="2" (
    echo.
    echo 启动Delve调试器...
    echo 使用方法:
    echo   - 输入 'c' 继续执行
    echo   - 输入 'n' 单步执行
    echo   - 输入 'b main.main' 在main函数设置断点
    echo   - 输入 'b 文件名:行号' 在指定位置设置断点
    echo   - 输入 'q' 退出调试
    echo.
    dlv debug ./cmd/nps/nps.go -- service
) else if "%choice%"=="3" (
    echo.
    echo 构建NPS服务...
    go build -o nps.exe cmd/nps/nps.go
    if exist nps.exe (
        echo 启动NPS服务...
        nps.exe service
    ) else (
        echo 构建失败！
    )
) else if "%choice%"=="4" (
    echo.
    echo 测试全局白名单功能...
    echo 当前配置:
    type conf\global.json
    echo.
    echo 启动服务进行测试...
    go run cmd/nps/nps.go service
) else (
    echo 无效选择！
)

pause