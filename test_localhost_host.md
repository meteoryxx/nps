# 本机服务器域名解析功能完整修复

## 问题描述
修复了"本机服务器在域名解析里用不了"的问题。
原因是域名解析功能缺少对本机客户端ID (-1) 的特殊处理，导致：
1. 前端没有提供"本机"选项
2. 后端无法正确获取本机客户端
3. **关键问题：没有自动设置LocalProxy=true，导致bridge尝试从不存在的客户端连接获取tunnel**

## 修改内容

### 1. 后端修复 (web/controllers/index.go)
- 在 `AddHost` 函数中，将 `file.GetDb().GetClient()` 改为 `s.getClientOrCreateLocalhost()`
- 在 `EditHost` 函数中，同样修改客户端获取逻辑
- **关键修复：在两个函数中添加检查本机客户端ID的逻辑，自动设置LocalProxy=true**
- 这样可以正确处理本机客户端ID (-1)

### 2. 前端修复 (web/views/index/hadd.html)
- 在 `getClientList()` 函数中添加"本机 (NPS服务器)"选项
- 设置默认选择本机选项，保持与隧道功能的一致性

### 3. 前端修复 (web/views/index/hedit.html)
- 将客户端ID输入框改为下拉选择框，便于用户选择
- 添加 `getClientList()` 函数来动态加载客户端列表，包括本机选项
- 正确设置当前选中的客户端ID，包括本机选项

## 测试步骤

### 1. 添加域名解析记录
1. 进入Web管理界面
2. 选择"域名解析" -> "添加"
3. 在客户端选择中应该能看到"本机 (NPS服务器)"选项
4. 选择本机选项，填写域名和目标地址
5. 保存后应该能成功创建

### 2. 编辑域名解析记录
1. 进入已有的域名解析记录编辑页面
2. 客户端应该显示为下拉选择框
3. 能够在本机和其他客户端之间切换
4. 保存修改应该成功

### 3. 验证功能
1. 创建本机域名解析记录
2. 配置域名指向NPS服务器
3. 通过域名访问应该能正常代理到本机服务

## 核心实现要点

### 1. 特殊客户端处理
- 使用 `getClientOrCreateLocalhost()` 方法处理特殊的本机客户端ID (-1)
- 创建虚拟的本机客户端对象，不存储到文件

### 2. 前端统一处理
- 在所有需要选择客户端的页面都提供"本机"选项
- 保持与隧道功能的一致性

### 3. 兼容性
- 现有的客户端功能不受影响
- 本机功能作为特殊客户端透明处理

## 核心问题和解决方案

### 问题根源
错误信息：`connect to target 127.0.0.1:8011 error the client -1 is not connect`

问题分析：
1. 在 bridge.go 的 SendLinkInfo 方法中，如果 LocalProxy=false，会尝试从 s.Client.Load(clientId) 获取客户端连接
2. 本机客户端ID (-1) 并不在 bridge 的 Client map 中，导致获取失败
3. 只有当 LocalProxy=true 时，才会直接使用 `net.Dial("tcp", link.Host)` 连接本地

### 解决方案
在域名解析的 AddHost 和 EditHost 函数中，添加与隧道功能相同的逻辑：
```go
// 判断是否为本机客户端，如果是则自动设置LocalProxy为true
localProxy := s.GetBoolNoErr("local_proxy")
if clientId == common.LOCALHOST_CLIENT_ID {
    localProxy = true
}
```

这样确保了：
- **统一处理**：域名解析功能现在与隧道功能保持一致，都支持本机选项
- **自动设置**：选择本机客户端时自动设置 LocalProxy=true，避免错误
- **代码复用**：重复使用已有的 `getClientOrCreateLocalhost()` 方法，保持代码一致性