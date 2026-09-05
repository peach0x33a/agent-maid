# agent-maid

把 **ChatGPT(Codex)** 与 **ZCode** 两个账号切换工具合并成一体的 TUI 管家。运行后首屏选择目标软件(官方图标渲染的渐变方块 logo,布局随终端宽度自适应并居中),再进入对应的账号切换器:

- **ChatGPT**:把各种已知格式的会话凭据(WebSession / sub2api 导出)导入统一账号库,一键投影为官方 Codex 原生 `auth.json`(`~/.codex/auth.json`),实现账号切换;官方 APP 零改动。
- **ZCode**:把当前 `~/.zcode/v2/credentials.json` 存进命名槽位,在多个账号之间一键切换(凭据密文原样搬运,自动解密出账号标签)。

Go 实现,单二进制。ChatGPT 部分完全离线(不联网刷新 token,不发送任何数据);ZCode 部分仅在内存中解密凭据用于展示标签,不落明文。

## 使用

```bash
go build -o agent-maid .
./agent-maid            # TUI:首屏选择 ChatGPT / ZCode
./agent-maid chatgpt    # 跳过首屏,直接进入 ChatGPT 账号管理
./agent-maid zcode      # 跳过首屏,直接进入 ZCode 账号管理
```

顶部始终显示 `AgentMaid - <平台>账号管理`;底部是操作栏(`←/→` 选中操作、回车执行,也可直接按对应键)。首屏选中的 logo 播放流光动画(渐变沿对角线循环流动,预渲染 24 帧),等待 ZCode 退出时有 braille 进度 spinner。两个平台共用同一套交互模式:

| 按键 | 作用 | ChatGPT | ZCode |
|---|---|---|---|
| `↑/↓` `j/k` | 移动列表光标 | ✔ | ✔ |
| `←/→` | 在底部操作栏中选择操作 | ✔ | ✔ |
| `enter` | 切换(激活选中账号/槽位) | ✔ | ✔ |
| `s` | 保存当前凭据(输入名字) | 保存当前 auth.json 到账号库 | 保存当前账号到新槽位 |
| `S` | 快存(自动命名) | ✔ | ✔ |
| `i` | 导入凭据 JSON | ✔ | — |
| `r` | 重命名选中项 | ✔ | ✔ |
| `d` | 删除选中项(**再按一次 d 确认**) | ✔ | ✔ |
| `m` | 信息 | 官方目录/账号库状态 | 机器信息(密钥指纹/deviceMid) |
| `?` | 帮助 | ✔ | ✔ |
| `Esc` | 返回目标软件选择 | ✔ | ✔ |
| `q` / `Q` | 退出 | ✔ | ✔ |

### ChatGPT 账号管理

- **导入**(`i`):输入 WebSession(`https://chatgpt.com/api/auth/session` 返回的 JSON)或 sub2api 导出的文件路径,也可以直接把 JSON 粘贴进输入框。解析是通用的(遍历所有「带 access_token 且带身份信息」的对象),导入只入库、不自动激活。
- **切换**(`enter`):把选中账号投影为官方原生 `auth.json`(键序与官方一致,未知键保留;合成可解析的占位 `id_token`;清掉 `OPENAI_API_KEY`)。原文件自动备份为 `auth.json.bak-<时间戳>`(保留最近 10 份),原子写入,权限 `0600`。
- **保存/快存**(`s`/`S`):把官方端当前生效的凭据(含官方端自行登录/刷新过的)收进账号库,按 `account_id` 去重更新。
- 账号库以 ChatGPT `account_id` 为主键;列表用 `●使用中` 标出当前 `auth.json` 实际生效的账号,`⚠无refresh_token` 提示过期后需重新导入。

### ZCode 账号管理

- **保存**当前凭据到命名槽位;**切换**时若当前账号未被任何槽位覆盖,先自动备份到 `auto-<时间戳>` 槽位。
- **ZCode 运行中的安全切换**:运行中替换可能被其退出前的凭据回写覆盖,因此默认**等待 ZCode 完全退出后自动执行**;需要强行操作时按 `f` 立即替换(无视风险)。
- 显示 `●使用中` 与 `⚠跨机/跨密钥` 警告;`m` 可查看密钥指纹、deviceMid 等机器信息。

## 配置与数据

统一存放在 `$AGENT_MAID_DIR`(缺省 `~/.config/agent-maid`):

```
~/.config/agent-maid/
├── chatgpt/store.json    # ChatGPT 账号库
└── zcode/slots/*.json    # ZCode 账号槽位
```

首次运行会自动把前身工具的数据搬过来(不覆盖已有文件):

- `~/.config/gptswitch/store.json` → `chatgpt/store.json`
- `~/.config/zcode-acc-switch/slots/*.json` → `zcode/slots/*.json`

## 环境变量

| 变量 | 作用 |
|---|---|
| `AGENT_MAID_DIR` | 覆盖统一配置目录 |
| `CODEX_HOME` | 覆盖官方 Codex 数据目录(缺省 `~/.codex`) |
| `ZCODE_V2_DIR` | 覆盖 ZCode 数据目录(缺省 `~/.zcode/v2`) |
| `ZCODE_ACC_SWITCH_DIR` | 覆盖 ZCode 槽位目录(设置后不做迁移,沿用旧目录) |
| `ZCODE_CREDENTIAL_SECRET` | 与 ZCode 本身一致:若 ZCode 以此变量运行,本工具会自动沿用 |

## 目录结构

```
main.go                    入口(可选参数 chatgpt / zcode 直接进入对应界面)
internal/meta/             程序名/版本/统一配置目录/旧数据迁移
internal/ui/               TUI 框架与各工具界面(见下)
internal/ui/manager.go     Manager 接口 + 工具注册表(接入点)
internal/ui/base.go        各工具共享的交互脚手架(模式/状态行/操作栏/删除待确认/摇晃)
internal/ui/root.go        根路由:首屏 ↔ 当前管理器(只面向接口)
internal/ui/select.go      首屏选择(单框自适应布局 + 流光渐变 logo)
internal/ui/logos.go       targetInfo 定义与半块渐变渲染(含动画帧缓存)
internal/ui/logos_art.go   官方图标栅格化的像素矩阵(生成文件,勿手改)
internal/ui/tui_chatgpt.go ChatGPT 管理器(实现 Manager)
internal/ui/tui_zcode.go   ZCode 管理器(实现 Manager)
internal/gptsession/       ChatGPT 凭据格式识别、JWT 解析、字段解析链、合成 id_token
internal/gptauth/          官方 auth.json 构建/合并(保序)、原子写入、备份
internal/gptstore/         ChatGPT 账号库(去重、激活指针、持久化 0600)
internal/zcode/            ZCode 凭据读取、AES-256-GCM 解密、机器信息、进程检测
internal/zcodestore/       ZCode 槽位存储(原子写入 0600)
internal/zcodeapp/         ZCode 业务操作(保存/切换/自动备份)
internal/atomicio/         临时文件 + rename 的原子写入
```

## 接入新工具

工具的凭据管理逻辑放独立的 internal 包(如 `gptauth`/`zcodeapp`),TUI 层实现
`ui.Manager` 接口(`Info/Chips/Init/Update/View`),然后在 `internal/ui/manager.go`
的 `newRegistry()` 里追加一行即可:首屏卡片、横幅、底部操作栏、删除待确认、
摇晃动画等全部自动复用,已有工具零改动。

## 安全说明

- ChatGPT:session JSON 里的 token 即登录凭证,导入后请删除源文件;账号库与 `auth.json` 为 `0600` 但仍是明文存储。本工具仅做本地格式转换与写盘,不能绕过 Codex OAuth 的认证限制。
- ZCode:槽位文件与凭据文件同为 `0600`,内容是密文原样搬运;仅在内存中解密用于展示标签。槽位跨机/跨用户搬运时若密钥指纹不一致将无法解密(界面会标注 ⚠),可在两边设置相同的 `ZCODE_CREDENTIAL_SECRET`。
