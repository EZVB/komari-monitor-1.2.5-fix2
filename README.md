# Komari Monitor 1.2.5-fix2

由 **HARVEY** 独立维护的 Komari 服务器监控版本。

本仓库以 Komari `1.2.5-fix2` 为固定起点，后续修复、功能调整和版本发布均基于本仓库的 `main` 分支进行，不以官方最新分支作为维护基线，也不会自动同步官方 `main`。

## 维护基线

| 项目 | 内容 |
| --- | --- |
| 维护者 | HARVEY |
| GitHub | [EZVB](https://github.com/EZVB) |
| 基线版本 | `1.2.5-fix2` |
| 基线提交 | `2f70b440405c4ea70ff3bcbd87361bbb39dc6f60` |
| 主维护分支 | `main` |
| 仓库地址 | [EZVB/komari-monitor-1.2.5-fix2](https://github.com/EZVB/komari-monitor-1.2.5-fix2) |

## 维护原则

- 所有后续开发均从本仓库的 `main` 分支开始。
- 不自动合并或跟随官方仓库的最新代码。
- 如需采用外部修复，将先独立评估，再作为本仓库自己的提交合入。
- 安装包、更新说明和版本标签以本仓库的 [Releases](https://github.com/EZVB/komari-monitor-1.2.5-fix2/releases) 为准。
- 已部署实例升级前应备份数据库和配置文件。

## Docker 镜像

本维护版唯一发布和维护的镜像地址为：

```text
ghcr.io/ezvb/komari:1.2.5-fix2
```

拉取固定版本：

```bash
docker pull ghcr.io/ezvb/komari:1.2.5-fix2
```

## 项目简介

Komari Monitor 是一套轻量、自托管的服务器监控程序，可通过 Web 页面查看服务器运行状态，并由客户端上报资源、网络和运行信息。

本维护版将围绕 `1.2.5-fix2` 持续修复问题和完善功能，避免因上游主线变化造成不必要的兼容性影响。

## 获取源码

```bash
git clone https://github.com/EZVB/komari-monitor-1.2.5-fix2.git
cd komari-monitor-1.2.5-fix2
git checkout main
```

## 本地构建

项目当前使用 Go `1.25.0`。

默认前端源码固定维护在本仓库的 `frontend/` 目录。GitHub Actions 会直接编译该目录，不会在构建时克隆或同步官方前端仓库。

```bash
go mod download
go build -o komari .
```

启动服务：

```bash
./komari server -l 0.0.0.0:25774
```

Windows 可将输出文件改为 `komari.exe`：

```powershell
go build -o komari.exe .
.\komari.exe server -l 0.0.0.0:25774
```

默认访问地址为 `http://服务器地址:25774`。

## 使用说明

本项目仅用于管理本人拥有或已获得明确授权的服务器。请勿将其用于未经授权的访问、控制或其他违规用途。

## 来源与许可

本仓库最初基于 [komari-monitor/komari](https://github.com/komari-monitor/komari) 的 `1.2.5-fix2` 版本建立。该链接仅用于记录原始来源，不代表本仓库会跟随其最新版本更新。

后续改动由 HARVEY 独立维护。项目授权方式以仓库中的 [LICENSE](./LICENSE) 文件为准。
