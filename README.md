# LytVPK

LytVPK 是面向 Left 4 Dead 2 的 Windows VPK 管理工具，支持 MOD 扫描、启用与禁用、创意工坊下载、服务器地图同步和快捷连接。

本项目完全由 AI 维护。

由 **菜且爱玩（ledslad）** 维护，基于 **LaoYutang** 的 [LytVPK 原项目](https://github.com/LaoYutang/lytvpk) 开发。感谢原作者的设计与实现。项目采用 [GPL-3.0-only](./LICENSE) 许可证。

[源码](https://github.com/ledslad/lytvpk) · [下载](https://github.com/ledslad/lytvpk/releases) · [问题反馈](https://github.com/ledslad/lytvpk/issues)

![LytVPK](https://img.shields.io/badge/Platform-Windows-blue)
![Build](https://img.shields.io/badge/Build-Wails_v2-green)
![Language](https://img.shields.io/badge/Language-Go_+_JavaScript-orange)
![License](https://img.shields.io/badge/License-GPL--3.0--only-blue)

## 本版本新增功能

- **服务器地图同步**：在独立的“麻衣安全屋”页面预览并同步地图，默认来源为 `cd.fastbot.top`；可在页面或网络设置中修改服务器。
- **同步预览与下载**：比对服务器完整清单、本机地图元数据和精确字节数，将缺图加入本机下载队列；缺少明确工坊 ID 的项目显示为需手动处理，不猜测下载。
- **工坊文件命名**：普通工坊 VPK 和同步地图统一使用实际下载条目的 `WorkshopID.vpk`，合集使用各子条目的 ID；普通下载更新已有 MOD 时保留原启用位置，文件名仍为 ID。压缩包等非 VPK 文件保留真实扩展名，直链下载保留独立命名规则。
- **可恢复清理**：仅按服务器明确的成功删除记录隔离本机旧地图；服务器当前仍保留的地图、依赖文件及其他 MOD 不会因为“不在列表”而被清理。支持恢复上次清理。
- **两服状态与快捷连接**：“麻衣安全屋”显示 `cd.fastbot.top` 战役服与 `gd.fastbot.top` 对抗服的状态、人数和地图，并提供快捷连接按钮。
- **公开只读接口接入**：地图清单与删除记录查询无需管理 token。遇到限流、查询失败或不完整快照时停止清理，下载提交与下载完成分别显示。

## 📖 使用文档

完整使用说明请查看：[https://lytvpk-docs.laoyutang.cn](https://lytvpk-docs.laoyutang.cn)

## 🚀 功能特性

### 核心功能
- **智能扫描**: 自动扫描和解析 VPK 文件，提取详细的内容信息
- **内容识别**: 智能识别地图、武器、角色、音频等游戏内容类型
- **标签系统**: 自动生成标签，支持按类型、位置、内容筛选
- **批量管理**: 支持批量启用/禁用 VPK 文件
- **文件导入**: 支持拖拽或选择文件导入 VPK/压缩包 到 addons 目录
- **创意工坊下载**: 支持解析创意工坊链接，直接下载并安装 Mod
- **服务器浏览器**: 支持查询服务器信息、玩家列表，一键连接服务器，收藏常用服务器
- **自动更新**: 启动时自动检测新版本，支持国内镜像源加速下载，一键无感更新

## 🛠️ 技术架构

### 后端 (Go)
- **框架**: Wails v2
- **VPK解析**: 使用 `l4d2-manager-next/pkg/valve/vpk` 和 `l4d2-manager-next/pkg/vpkmission`
- **并发处理**: `github.com/panjf2000/ants/v2` 协程池
- **配置管理**: JSON 格式的持久化配置

### 前端 (JavaScript + CSS)
- **原生 JavaScript**: 无框架依赖，轻量高效
- **现代 CSS**: 基于 CSS 变量的设计系统
- **响应式设计**: 支持桌面端和移动端
- **实时通信**: 通过 Wails 事件系统与后端通信

## 📦 安装和使用

### 系统要求
- Windows 10/11

### 使用说明
1. **选择目录**: 点击"选择L4D2目录"按钮，选择游戏的 addons 文件夹
2. **扫描文件**: 应用会自动扫描并解析所有 VPK 文件
3. **管理插件**: 使用界面上的开关来启用/禁用插件
4. **筛选搜索**: 使用搜索框和标签筛选来查找特定插件
5. **批量操作**: 选择多个文件进行批量启用/禁用
6. **导入文件**: 点击上传按钮或直接拖拽文件到窗口即可导入
7. **下载 Mod**: 输入创意工坊链接，点击下载即可自动安装
8. **服务器连接**: 在服务器页面添加 IP，查看状态并一键连接
9. **版本更新**: 应用启动会自动检查更新，发现新版本会提示升级

## 🙏 致谢

- [Wails](https://wails.io/) - 跨平台应用框架
- [l4d2-server-next](https://github.com/LaoYutang/l4d2-server-next/tree/master/backend/pkg) - VPK 与 mission 文件解析库
- [ants](https://github.com/panjf2000/ants) - 高性能协程池

## 📄 开源协议

本项目采用 [GNU General Public License v3.0 only](./LICENSE) 许可证。第三方依赖与资源遵循各自许可证。
