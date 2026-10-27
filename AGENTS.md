# LytVPK 知识库

**Generated:** 2026-05-26
**领域:** Windows 桌面端 VPK Mod 管理器（Left 4 Dead 2）
**核心栈:** Go 1.26.10 + Wails v2.10.2 + Vite 6 + Node 24 + 原生 JavaScript/CSS

## 概述

基于 Wails v2 的 Windows 桌面应用。Go 后端负责文件系统、VPK 解析、网络请求和系统集成；前端原生 JavaScript + CSS 实现 UI。核心能力：VPK 扫描解析、内容类型识别（地图/人物/武器）、标签管理、创意工坊下载、服务器浏览器、应用更新。

## 结构

```
.
├── main.go                    # Wails 入口：嵌入 frontend/dist、绑定 App、单例检测
├── wails.json                 # Wails 构建配置
├── go.mod                     # Go 1.26.10，模块 vpk-manager
├── internal/
│   ├── app/                   # 后端业务层，208 个 Wails 导出方法
│   │   └── AGENTS.md         # → 后端业务逻辑详细指南
│   ├── parser/                # VPK 解析与内容识别引擎
│   │   └── AGENTS.md         # → 解析器详细指南
│   ├── network/               # Steam 图片代理、IP 优选
│   └── platform/              # Windows 注册表、URL Protocol
├── frontend/
│   ├── src/js/
│   │   ├── main.js            # JS 入口
│   │   ├── core/              # UI shell、主题、toast、通用工具
│   │   └── features/          # 业务功能模块
│   │       └── AGENTS.md     # → 前端功能模块详细指南
│   ├── src/css/               # 全局/布局/功能样式
│   ├── wailsjs/               # Wails 自动生成绑定，禁止手动修改
│   └── index.html             # HTML 入口（含完整 UI 结构）
├── build/                     # Wails 构建资源、图标
├── worker/                    # Steam Workshop 外部脚本
└── .github/workflows/         # Release CI，推送 v* tag 触发
```

## 查询指南

| 任务                   | 位置                            | 备注                                      |
| ---------------------- | ------------------------------- | ----------------------------------------- |
| 修改后端业务逻辑       | `internal/app/*.go`             | 参考 `internal/app/AGENTS.md`             |
| 修改 VPK 解析/类型识别 | `internal/parser/*.go`          | 参考 `internal/parser/AGENTS.md`          |
| 修改前端 UI/交互       | `frontend/src/js/features/*.js` | 参考 `frontend/src/js/features/AGENTS.md` |
| 修改图片代理/IP 优选   | `internal/network/*.go`         | 本地 HTTP 服务器绕过 Steam CORS           |
| 修改 Windows 平台集成  | `internal/platform/*.go`        | URL Protocol、注册表操作                  |
| 修改构建配置           | `wails.json`                    | 前后端构建命令定义                        |
| 修改 CI/发布流程       | `.github/workflows/release.yml` | windows-latest，Go 1.26.10，Node 24       |

## 核心符号

| 符号             | 类型   | 位置                                           | 职责                              |
| ---------------- | ------ | ---------------------------------------------- | --------------------------------- |
| `App`            | struct | `internal/app/app.go`                          | Wails 绑定主结构，208 个导出方法  |
| `ParseVPKFile`   | func   | `internal/parser/parser.go`                    | VPK 解析主入口，单次遍历提取资源  |
| `VPKFile`        | struct | `internal/parser/types.go`                     | 前后端传输核心数据结构            |
| `ScanVPKFiles`   | func   | `internal/app/vpk_scan.go`                     | 扫描缓存逻辑，四重校验            |
| `createFileItem` | func   | `frontend/src/js/features/file-list/render.js` | 列表项 DOM 渲染                   |
| `appState`       | object | `frontend/src/js/features/state.js`            | 全局状态：vpkFiles、selectedFiles |

## 约定

- **无前端框架**: 原生 JavaScript + CSS 变量，禁止引入 React/Vue/Svelte
- **功能模块化**: 新功能放入 `frontend/src/js/features/` 对应子目录，禁止全局逻辑
- **文件命名**: Go 按功能前缀分组 `workshop_*.go`、`vpk_*.go`、`panel_*.go`
- **平台隔离**: `secret_windows.go` / `secret_other.go` 用构建标签分离
- **路径操作**: 使用 `filepath.Join`，优先考虑 Windows 行为
- **并发管理**: 优先使用 `a.goroutinePool.Submit()`，禁止裸 `go func()`
- **状态驱动**: 前端 `state.js` 是单一数据源，修改后触发重新渲染

## 反模式

### 禁止修改
- `frontend/wailsjs/**` — Wails 自动生成，会被覆盖
- `build/windows/installer/*.nsh` — Wails 构建自动生成
- `frontend/dist/**` — Vite 构建产物

### 禁止新增
- ❌ 裸 goroutine — 使用协程池
- ❌ `innerHTML` — 使用 `createElement`/`textContent`
- ❌ `console.log` — 生产代码移除调试日志
- ❌ 全局逻辑 — 放入对应 feature 模块
- ❌ 临时字符串解析 — 复用 `internal/parser`
- ❌ 字符串拼接路径 — 使用 `filepath.Join`

### 安全警告
- ⚠️ 后端操作会移动/删除用户 Mod 文件，必须保留清晰错误处理
- ⚠️ `unsafe.Pointer` 仅允许在 `secret_windows.go`（Windows DPAPI）
- ⚠️ 修改 `VPKFile` 结构后必须同步更新前端类型定义和重新生成 Wails 绑定

## 命令

```powershell
# 本地开发
wails dev

# 验证构建（沙箱中可能报 Access is denied，直接提权运行）
wails build

# 发布构建（CI 自动执行）
wails build -platform windows/amd64 -ldflags "-X main.AppVersion=<version>" -o "LytVPK MOD管理器.exe"

# 运行测试（通过不代表完整集成验证）
go test ./...

# Go 格式化
gofmt -w <file>
```

## 注意事项

- `main.AppVersion` 默认 `"0.0.0"`，发布时通过 `-ldflags` 注入
- 配置存储在 `%APPDATA%/LytVPK/config.json`，当前迁移版本 v5；新增 `mapSyncServer` 默认指向 `cd.fastbot.top`，在上游排序与 DNS 配置迁移基础上补齐默认值，保留已有设置。
- 创意工坊、图片代理、服务器查询依赖外部网络，受限环境可能无法验证
- 单例通过 TCP 端口 `127.0.0.1:19527` 实现，支持 URL Protocol 深度链接 (`lytvpk://`)
- 前端 `index.html` 1200+ 行、`servers.js` 971 行、`app-runtime.js` 906 行 — 大文件现状，修改时保持局部

## 上游合并、开源与双仓库同步

- 上游为 `LaoYutang/lytvpk`；本二次开发项目公开仓库为 `https://github.com/ledslad/lytvpk`，公开分支为 `main`。维护署名使用“菜且爱玩（ledslad）”，保留 LaoYutang 的原作者版权、原项目链接和 GPL-3.0-only 许可证，不将上游作品声明为本 fork 独立原创。
- 合并上游时记录精确提交，使用共同基线进行三方合并，保留本地地图同步、服务器入口和已有定制；发生冲突时核对行为，不能整文件选边覆盖。Wails 绑定与前端 dist 必须通过工具重新生成。
- 私有主仓库与公开 GitHub 仓库分别同步：私有主仓库保留项目所在目录并推送 `origin/master`；公开仓库仅接收经过审查的该项目源码，推送独立仓库的 `main`。不得把包含其他项目、备份、部署配置的私有主仓库或其历史直接推到公开远端。
- 每次公开发布前扫描完整待发布文件、提交元数据及构建产物，排除真实 token、密码、Steam 登录凭据、服务器私钥、用户私有邮箱、个人备案号、备份、操作日志、运行时数据库和本机部署路径。示例配置使用占位符；不得为了通过扫描而删除原作者已公开的版权信息。
- 本 fork 可公开的服务器连接预设为用户明确允许的 `cd.fastbot.top` 与两服状态/快捷连接功能所需的 `gd.fastbot.top`。这不授权发布管理接口凭据、RCON 密码或服务器运行环境文件。
- 二进制版本必须可对应到公开源码提交或标签；公开发行只使用本 fork 的 GitHub Releases 和问题反馈地址，不替换原作者官方发行内容。发布前执行 Go 测试、前端测试与构建，并验证不带管理 token 的公开同步接口行为。
- 私有维护工作区从项目目录运行 `../project-sync/Sync-Project.ps1 -Project lytvpk`，先推私有仓库并生成白名单导出；检查真实文件清单与零发现隐私报告后，按 `../project-sync/README.md` 携带清单哈希完成 GitHub 同步。公共源码仓库不包含该私有工作区工具，普通贡献者直接使用该独立仓库的 Git 流程。
- 交付时分别报告私有提交与公开提交/标签，不能把只完成其中一个仓库的同步表述为双同步完成。
- Wails v2.10.2 绑定生成使用与 CI 相同的 Go 1.26.10（可设置 `GOTOOLCHAIN=go1.26.10`）；更高版本 Go 出现标准库类型载入错误时不要跳过绑定生成，应使用匹配工具链重建。
- 收藏服务器保留上游布局和最近服务器行为；社区两服状态、连接及地图同步集中在独立“麻衣安全屋”页面（`features/safehouse/`），不得再次注入收藏页。同步预览、明确删除记录及隔离恢复规则必须保留。
- README 使用简洁产品介绍，明确“本项目完全由 AI 维护”，保留维护者、原作者鸣谢及许可证；配置细节、合并记录和维护操作放对应文档，私人配置只留私有文档及运行配置。
- 用户要求自行确认界面：默认只编辑源码、执行自动测试和构建，不操作桌面窗口、鼠标或键盘。除非用户之后明确要求，不为验收自动启动、关闭或接管应用。
- 地图同步按每个 VPK 的精确地图 ID/工坊 ID 和服务器明确删除记录处理，包括不含 missions/章节的资源分卷；不按标题推断关联，不连带删除未记录依赖，保留当前服务器仍安装的 VPK。清理移入 `addons/disabled/server-sync-*`，回收站仅按有效恢复记录处理普通文件，禁止递归删除整个 disabled，禁止清理 disabled 根目录的手动禁用文件。清空前显示 VPK 数量、文件数量、空间及永久删除确认，并在后端重核目录和文件快照；异常批次保留并显示具体原因，不能在有跳过批次时只显示“回收站为空”或清理成功，活动下载时拒绝清空。
- Wails 使用 `bindings` 构建标签执行主程序生成绑定。`singleton_bindings.go` 跳过单例转发，确保应用正在运行时仍可生成新绑定；运行版本的单例行为由 `singleton_runtime.go` 保持。
- 连接服务器时，域名先通过本机 DNS 解析为数字 IP（优先 IPv4，保留端口），再交给 `steam://connect/`。Steam 可能接收域名协议链接但不启动游戏，不能直接透传域名。域名解析需限时，协议打开错误必须返回界面；调用成功不能表述为游戏已连入。
- 自动更新在用户配置目录暂存、校验并备份，主程序正常退出后由专用助手覆盖现有 EXE，保留文件 ACL；不要依赖安装目录能创建 `.new`/`.old`。助手入口在单例和 Wails 初始化前处理，不能打开界面或强杀主进程。用户确认更新重启时由助手在安装结束后重开，普通退出只安装。覆盖失败保留或回退备份，并在下次检查更新时报告。
