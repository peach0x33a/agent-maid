# Agent Maid

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)](#平台支持)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](#许可)

**Agent Maid** 是一个终端账号管家（TUI）：把多个 AI 工具的账号凭据集中管理，在多个账号之间一键切换。

运行后首屏选择目标软件（官方图标渲染的流光渐变 logo），进入对应的账号管理器。目前支持：

| 工具 | 管理对象 | 机制 |
|---|---|---|
| **ChatGPT（Codex）** | `~/.codex/auth.json` | 导入多格式会话凭据建立账号库，一键投影为官方 Codex 原生 `auth.json` |
| **ZCode** | `~/.zcode/v2/credentials.json` | 账号槽位：凭据密文原样快照，一键切换 |
| **Antigravity** | `~/.gemini/antigravity-cli/antigravity-oauth-token` | agy CLI 登录凭据快照与恢复 |

全部操作**纯本地、不联网**（Antigravity/ChatGPT 不刷新 token，有 `refresh_token` 的账号由官方端自行续期）。

## 特性

- 🖥 **单二进制 TUI**：Go 实现，无外部运行时依赖，交叉编译即得三大平台
- 🎨 **终端自检向导**：首次运行两步自检（渐变色/特殊符号），符号不支持时自动降级为纯文本模式
- 🧩 **插件式架构**：新工具只需实现一个 `Manager` 接口 + 一行注册，首屏/横幅/操作栏/动画全部复用
- ⌨️ **统一交互**：所有平台同一套按键（保存/切换/重命名/删除/撤销/登出/信息），底部操作栏可方向键导航
- 🛟 **防误触设计**：删除需双击确认（可 `z` 撤销）、退出需双击 Q（间隔 ≥0.5s）、切换需二次 Enter、目标软件运行中有明确警告
- 🏠 **数据统一存放**：`~/.config/agent-maid/`（Windows 为 `%AppData%\agent-maid`），自动迁移前身工具的数据

## 安装

从源码构建（需要 Go 1.26+）：

```bash
go build -o agent-maid .
```

或交叉编译到其他平台：

```bash
GOOS=darwin  GOARCH=arm64 go build -o agent-maid-macos
GOOS=windows GOARCH=amd64 go build -o agent-maid.exe
```

### 预编译产物

打上 `v*` 形式的 tag 并推送后，GitHub Actions 会跑测试、构建
Linux/macOS 的 amd64+arm64 二进制，并发布到 Releases：

```bash
git tag v1.0.0
git push origin v1.0.0
```

下载的文件形如 `agent-maid_1.0.0_linux_amd64`（静态链接，可直接运行），
每个都附带 `.sha256` 校验和。二进制里的版本号取自 tag，可用
`agent-maid version` 核对。

## 快速上手

```bash
./agent-maid              # 首次运行:设置向导 → 首屏选择目标软件（按 S 打开设置）
./agent-maid chatgpt      # 跳过首屏,直接进入 ChatGPT 账号管理
./agent-maid zcode        # 跳过首屏,直接进入 ZCode 账号管理
./agent-maid antigravity  # 跳过首屏,直接进入 Antigravity 账号管理
./agent-maid version      # 显示版本
./agent-maid --help       # 显示用法
```

目标名不区分大小写,也接受简写 `gpt` / `z` / `anti` / `ag`。
TUI 需要交互式终端:在管道或脚本里运行会直接提示并以状态码 2 退出。

顶部始终显示 `AgentMaid - <平台>账号管理`；底部是操作栏（`←/→` 选中操作、回车执行，也可直接按对应键）。

### 按键说明

| 按键 | 作用 | ChatGPT | ZCode | Antigravity |
|---|---|:---:|:---:|:---:|
| `↑/↓` `j/k` | 移动列表光标 | ✔ | ✔ | ✔ |
| `←/→` | 在底部操作栏中选择操作 | ✔ | ✔ | ✔ |
| `enter` | 切换（再按一次 Enter 确认，提示显示在状态行） | ✔ | ✔ | ✔ |
| `s` | 保存当前凭据（默认填充名称） | 保存当前 auth.json | 保存当前账号到新槽位 | 保存当前账号到新槽位 |
| `i` | 导入凭据 JSON（支持一文件多账号/JSONL） | ✔ | — | — |
| `r` | 重命名选中项 | ✔ | ✔ | ✔ |
| `d` | 删除选中项（再按一次 d 确认，5s 不动自动撤下，移入回收站） | ✔ | ✔ | ✔ |
| `m` | 信息（目录/账号/密钥指纹/deviceMid） | ✔ | ✔ | ✔ |
| `n` | 登出：清空本地凭据（账号自动保留，令牌不吊销），用于重新登录新账号 | ✔ | ✔ | ✔ |
| `z` | 撤销最近一次删除（回收站，退出前有效） | ✔ | ✔ | ✔ |
| `?` | 帮助（`↑/↓` 滚动） | ✔ | ✔ | ✔ |
| `Ctrl+R` | 重新读盘（软件刚登录/登出后在 TUI 内刷新） | ✔ | ✔ | ✔ |
| `Esc` | 返回目标软件选择 | ✔ | ✔ | ✔ |
| `q` / `Q` | 退出（双击确认，反选高亮提示，间隔 ≥0.5s） | ✔ | ✔ | ✔ |

### 无头命令（脚本 / AI agent）

不需要交互终端，每个命令都支持 `--json`：

```bash
./agent-maid list    chatgpt --json     # 列出账号，标出正在使用的
./agent-maid current zcode   --json     # 现在登录的是谁
./agent-maid use     antigravity work   # 切换/激活到指定槽位
./agent-maid save    chatgpt [名称]      # 把当前生效的凭据存进账号库
./agent-maid rm      zcode work         # 删除（移入回收站）
./agent-maid import  chatgpt -          # 从 stdin 导入 Codex 会话 JSON
./agent-maid logout  chatgpt            # 登出（账号保留，令牌不吊销）
```

退出码：`0` 成功、`1` 执行失败、`2` 用法错误。`--json` 时错误也输出为
`{"type":"error","ok":false,"error":"…"}`，便于 agent 统一解析。

`use` 默认**拒绝**从"尚未保存的当前账号"切走：否则原账号只剩一份没人认得出
的 `auto-<时间戳>` 备份。按提示先 `save` 再切；确认当前登录可丢弃时加
`--force`（仍会备份）。

`import` 只对 ChatGPT 有效——ZCode 与 agy 没有可导入的文件格式，账号只能从
本机已登录状态用 `save` 收录。

### 给 AI agent 用：skills

仓库自带一个 agent skill：

```
skills/agent-maid/
├── SKILL.md              # 入口：何时用、核心流程、注意事项
└── references/
    ├── cli.md            # 每个命令与 JSON 字段（实测对齐）
    └── state.md          # 数据布局、环境变量、排障
```

安装到你的 agent 的 skills 目录即可，例如：

```bash
cp -r skills/agent-maid ~/.agents/skills/     # 或 ~/.claude/skills/
```

## 各工具说明

### ChatGPT 账号管理

- **导入**（`i`）：支持 WebSession（`https://chatgpt.com/api/auth/session` 返回的 JSON）、sub2api/CPA 导出整包，或直接粘贴 JSON——解析是通用的，遍历所有「带 access_token 且带身份信息」的对象；**一文件多账号**（数组、整包、每行一个账号的 JSONL 均可，允许混排与夹杂无效行）。导入只入库、不自动激活。
- **激活**（`enter`，二次确认）：把选中账号投影为官方 Codex 原生 `auth.json`——键序与官方一致、未知键保留、合成可解析的占位 `id_token`、清掉 `OPENAI_API_KEY`；原子写入，权限 `0600`。
- **保存**（`s`）：把官方端当前生效的凭据（含官方端自行刷新过的）收进账号库，按 `account_id` 去重更新。
- 账号库以 ChatGPT `account_id` 为主键去重；列表用 `●使用中` 标出当前 `auth.json` 实际生效的账号，`⚠无refresh_token` 提示过期后需重新导入。

### ZCode 账号管理

- 保存当前凭据到命名槽位；切换时若当前账号未被任何槽位覆盖，先自动备份到 `auto-<时间戳>` 槽位。
- **安全切换**：ZCode 运行中替换可能被其退出前的凭据回写覆盖，因此默认**等待 ZCode 完全退出后自动执行**（二次 Enter 确认后进入等待）；等待中可按 `f` 无视风险立即替换、按 `c` 关闭本工具交由后台进程继续等待并在完成后弹系统通知。
- **登出**（`N`）：清空本地凭据让 ZCode 回到未登录态，用于重新登录新账号——避免在软件内点「退出登录」导致服务端吊销令牌；当前账号会先自动备份。
- 显示 `使用中` 徽标与 `⚠跨机/跨密钥` 警告；`m` 可查看密钥指纹、deviceMid 等机器信息。

### Antigravity（agy CLI）账号管理

- 管理的是 **agy（Antigravity CLI）** 的登录凭据：`~/.gemini/antigravity-cli/antigravity-oauth-token`（单个 JSON，0600，紧凑单行），不是桌面端那个从未存在过的 `~/.gemini/oauth_creds.json`。
- 槽位保存该文件原文；切换即原子写回，写回格式与 agy 自身完全一致（紧凑 JSON、无结尾换行、`0600`），因此 agy 直接可用。槽位标签（邮箱）取文件里的 `id_token_email`，缺失时回退到 `id_token` JWT 声明。
- 凭据里有 `refresh_token` 时过期由 agy 自动续期，界面标注「可自动续期」；没有则标注 `⚠无refresh_token`。
- agy 正在运行时会提示：切换后建议尽快重启 agy，避免运行中的凭据回写覆盖。
- **登出**（`N`）：把凭据文件改名为 `.antigravity-oauth-token.logged-out-<时间>`，与 agy 自己的登出行为一致（误登出还能找回），当前账号自动备份到槽位。

## 配置与数据

统一存放在 `$AGENT_MAID_DIR`（缺省见平台支持表）：

```
~/.config/agent-maid/
├── settings.json               # 向导结果与界面偏好
├── chatgpt/store.json          # ChatGPT 账号库
├── zcode/slots/*.json          # ZCode 账号槽位
└── antigravity/slots/*.json    # Antigravity 账号槽位
```

首次运行会自动把前身工具的数据搬过来（只迁移一次，不覆盖已有文件）：

- `~/.config/gptswitch/store.json` → `chatgpt/store.json`
- `~/.config/zcode-acc-switch/slots/*.json` → `zcode/slots/*.json`

### 环境变量

| 变量 | 作用 |
|---|---|
| `AGENT_MAID_DIR` | 覆盖统一配置目录 |
| `CODEX_HOME` | 覆盖官方 Codex 数据目录（缺省 `~/.codex`） |
| `ZCODE_V2_DIR` | 覆盖 ZCode 数据目录（缺省 `~/.zcode/v2`） |
| `ZCODE_ACC_SWITCH_DIR` | 覆盖 ZCode 槽位目录（设置后不做迁移，沿用旧目录） |
| `ZCODE_CREDENTIAL_SECRET` | 与 ZCode 本身一致：若 ZCode 以此变量运行，本工具会自动沿用 |
| `ANTIGRAVITY_GEMINI_DIR` | 覆盖 Antigravity 的 gemini 数据目录（缺省 `~/.gemini`） |
| `ANTIGRAVITY_CLI_DIR` | 覆盖 agy 数据目录（缺省 `$ANTIGRAVITY_GEMINI_DIR/antigravity-cli`） |
| `ANTIGRAVITY_STORE_DIR` | 覆盖 Antigravity 槽位目录（缺省 `~/.config/agent-maid/antigravity/slots`） |

## 平台支持

| | Linux | macOS | Windows |
|---|---|---|---|
| 编译 | ✓ | ✓ (amd64/arm64) | ✓ (amd64/arm64) |
| TUI / 切换功能 | ✓ | ✓ | ✓ |
| 配置目录 | `~/.config/agent-maid` | `~/.config/agent-maid` | `%AppData%\agent-maid` |
| 运行检测（/proc、ps、tasklist） | ✓ | 尽力而为¹ | ✓ |
| ZCode「等待退出后切换」 | ✓ | 同左（依赖进程检测） | 同左（依赖进程检测） |

¹ macOS 通过 `ps` 还原 argv 属尽力而为，不保证兼容性。Windows 上文件权限位（0600）不映射 NTFS ACL，建议将配置目录放在受保护的账户下；TUI 建议在 Windows Terminal 中运行。

## 项目结构

```
main.go                    入口（TUI 启动参数 / 无头命令；--wait-apply 后台切换）
internal/meta/             程序名/版本/统一配置目录/设置/旧数据迁移
internal/ui/               TUI 框架与各工具界面（见下）
internal/ui/manager.go     Manager 接口 + 工具注册表（接入点）
internal/ui/base.go        各工具共享的交互脚手架（模式/状态行/操作栏/删除待确认/摇晃）
internal/ui/root.go        根路由：向导 → 首屏 ↔ 当前管理器（只面向接口）
internal/ui/select.go      首屏选择（单框自适应布局 + 流光渐变 logo）
internal/ui/wizard.go      首次运行设置向导
internal/ui/logos.go       targetInfo 定义与半块渐变渲染（含动画帧缓存）
internal/ui/logos_art.go   官方图标栅格化的像素矩阵（生成文件，勿手改）
internal/ui/tui_chatgpt.go ChatGPT 管理器（实现 Manager）
internal/ui/tui_zcode.go   ZCode 管理器（实现 Manager）
internal/ui/tui_antigravity.go  Antigravity 管理器（实现 Manager）
internal/gptsession/       ChatGPT 凭据格式识别、JWT 解析、字段解析链、合成 id_token
internal/gptauth/          官方 auth.json 构建/合并（保序）、原子写入、备份
internal/gptstore/         ChatGPT 账号库（去重、激活指针、持久化 0600）
internal/zcode/            ZCode 凭据读取、AES-256-GCM 解密、机器信息
internal/zcodestore/       ZCode 槽位存储（原子写入 0600）
internal/zcodeapp/         ZCode 业务操作（保存/切换/自动备份）
internal/antigravity/      agy 凭据读写、槽位存储
internal/procscan/         跨平台进程扫描（/proc、ps、tasklist）
internal/atomicio/         临时文件 + rename 的原子写入
internal/notify/           系统级桌面通知（notify-send / osascript / MessageBoxW）
internal/background/       脱离终端的后台等待切换进程
internal/cli/              无头命令（list/current/use/save/rm/import/logout）
internal/gptapp/           ChatGPT 业务操作（TUI 与 CLI 共用的唯一实现）
skills/agent-maid/         agent skill（入口 SKILL.md + references/）
```

## 接入新工具

工具的凭据管理逻辑放独立的 internal 包（如 `gptauth`/`zcodeapp`），TUI 层实现
`ui.Manager` 接口（`Info`/`Chips`/`Init`/`Update`/`View`），然后在
`internal/ui/manager.go` 的 `newRegistry()` 里追加一行即可：首屏卡片、横幅、
底部操作栏、删除待确认、摇晃动画等全部自动复用，已有工具零改动。

## 安全说明

- **ChatGPT**：session JSON 里的 token 即登录凭证，导入后请删除源文件；账号库与 `auth.json` 为 `0600` 但仍是明文存储。本工具仅做本地格式转换与写盘，不能绕过 Codex OAuth 的认证限制。
- **ZCode**：槽位文件与凭据文件同为 `0600`，内容是密文原样搬运；仅在内存中解密用于展示标签。槽位跨机/跨用户搬运时若密钥指纹不一致将无法解密（界面会标注 ⚠），可在两边设置相同的 `ZCODE_CREDENTIAL_SECRET`。
- **Antigravity**：快照文件包含长期有效的 `refresh_token`，请像对待密码一样对待槽位文件。
- 本工具不联网、不上传任何数据；请在遵守对应服务条款的前提下使用。

## 许可

[MIT](LICENSE)
