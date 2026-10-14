# Equus

Equus 是一个主要仿照 Codex（OpenAI 终端 Coding Agent）架构搭建的极简终端 Coding Agent，使用 Go 编写。主链为「配置加载 → LLM 客户端 → Agent 循环 → 工具执行 → TUI」；交互模式是完整的全屏 TUI，并已支持会话持久化/resume、审批策略、上下文压缩、MCP、Skill、子代理等能力。外层架构参照 Codex，文件工具契约与 TUI 参照 Claude Code。

Equus（马属）—— 一匹小马。

## 为什么用 Go

Codex 用 Rust 写，Claude Code 用 TypeScript 写，Equus 选择 Go，理由很朴素：

- **Rust**：性能和内存安全无可挑剔，但所有权、生命周期这套编译期约束学习成本高，改一处代码常常要连锁改十处，不适合快速迭代一个还在成长中的框架。
- **TypeScript**：生态最丰富、上手最快，但产物依赖 Node 运行时，发给别人不是「一个文件」；调试 agent 主循环时层层异步回调的调用栈也不够直观。
- **Go**：`go build` 直接产出静态链接的单文件二进制，**无需 CGO 即可交叉编译到 Linux/macOS/Windows**；并发就是一个 `go` 关键字；标准库自带 HTTP/JSON，核心依赖精简（LLM 域仅 cobra + yaml，TUI 引入 charmbracelet 系列）；类型系统「够用就好」，代码读起来接近直白的英文，重构很快。

一句话：在「够快的性能」和「够快的开发速度」之间取中庸，Go 最省心。

## 功能

- 与多种 wire 端点对话：OpenAI Chat Completions（OpenAI、GLM、DeepSeek、Qwen 等）、OpenAI Responses API、Anthropic Messages（Claude 及兼容端点）。由 provider 的 `wire_api` 选择。
- SSE 流式输出，实时显示思考过程（reasoning）、工具调用与 token 用量。
- 多轮对话记忆。
- 内置文件工具（限制在工作目录内）：
  - `read_file` / `write_file` / `list_files` —— 读写与列目录；
  - `apply_patch` —— 以补丁文本（`*** Begin Patch` … `*** End Patch`）新增 / 删除 / 更新 / 移动文件，支持多块编辑与模糊上下文定位；
  - `glob` —— 按 glob 模式查找文件（`**` 递归）；
  - `grep` —— 按正则搜索文件内容，返回 `path:line: text`；
  - `shell` —— 在工作目录执行 shell 命令（默认需审批）；
  - `web_fetch` —— 抓取公开 HTTP(S) URL 的可读文本（拒绝私网/环回地址）。
- **MCP 工具**：连接配置的 MCP server（stdio），把其工具动态注册为 `mcp__<server>__<tool>`（默认需审批）。
- **子代理**：`spawn_agent` / `wait_agent` / `close_agent` —— 异步派生**只读**子代理执行子任务（最多 8 个，禁递归），完成后主动通知主会话。
- **Skill**：启动时发现 `SKILL.md` 技能（`~/.equus/skills/` 与 `<cwd>/.equus/skills/`），在系统提示词中列出供模型按需读取。
- **审批策略**：高危操作按类别（`shell` / `write` / `network` / `external`）决定询问 / 放行 / 拒绝。`shell_allow_prefixes` 命令前缀白名单自动放行（默认含 `ls`/`cat`/`git status` 等只读命令）。交互模式弹窗确认（`[y]` 一次 / `[a]` 本会话 / `[p]` 永久 / `[n]` 拒绝），`[p]` 写入 `~/.equus/approvals.yaml`；非交互 `exec` 默认拒绝，`--allow-shell` 放行。
- **自动压缩（compaction）**：上下文接近窗口上限（默认 80%）时，自动把较早历史摘要成一段、保留最近若干轮原文；状态行显示 `ctx N%`，`/status` 显示本次会话累计 token。
- **系统提示词**：自动注入工作目录、平台与工具使用引导；若项目内含 `AGENTS.md`（项目根到工作目录逐层），其内容会一并注入并声明优先。可在配置中用 `base_instructions` 完全覆盖。
- YAML 配置 + 环境变量覆盖（`${VAR}` 展开与 `EQUUS_*` 变量）。
- API Key 支持三种注入方式：配置文件内联 / 密钥文件（`api_key_file`）/ 环境变量。
- **交互式 TUI**（Bubble Tea，全屏）：启动字标、自绘可滚动对话区 + 滚动条、行编辑输入框（多行、命令补全）、状态行（模型 / cwd / git 分支 / ctx% / 状态）、实时思考与工具调用显示。
- 助手输出支持 **Markdown 渲染**（标题/列表/引用/表格/行内样式），代码块用 chroma 语法高亮。
- **Slash 命令**：`/help`、`/clear`、`/compact`、`/model`、`/status`、`/resume`、`/rename`、`/delete`、`/quit`（`/exit` 同义）。
- **会话持久化**：append-only JSONL，支持 `--continue`（续最近）、`--resume <id>`、`equus --resume`（交互式选择器）；`/rename`、`/delete` 管理当前会话。

## 设计来源

### 主要仿照 Codex

- **分层架构与依赖方向**：`cmd` 进程入口 → `cli` 命令层 → `config` 配置 → `llm` Provider 中立模型域 → `llm/openai` 传输适配器 → `agent` 主循环。依赖单向、领域与传输分离，与 Codex 的 package 组织方式一致。
- **Provider 中立模型域**：`Request`/`Response`/`ResponseItem`/`ToolCall`/`Client`/`Stream` 等中立类型，模型选择与传输配置解耦（`model_providers` + `wire_api` 抽象）。
- **Agent 主循环**：采样 → 解析工具调用 → 执行 → 工具结果回填 → 继续采样直到 `finish_reason=stop`；流式优先、失败自动回退非流式。
- **Chat Completions SSE 流式**：逐分片解析，包括跨分片的 tool call 参数拼接与 usage 汇总。
- **配置体系**：YAML + `${VAR}` 环境引用展开 + `EQUUS_*` 环境变量覆盖 + 校验与密钥脱敏；密钥可内联、走密钥文件（`api_key_file`）或环境变量。

### 参照了 Claude Code

- **工具契约**：每个工具自带名称、自然语言描述和 JSON Schema 参数定义，由模型自主决定何时调用。
- **文件工具集与安全默认值**：`read_file` / `write_file` / `list_files` / `apply_patch` / `glob` / `grep` 六个文件工具，所有路径限制在工作目录内（`resolvePath` 逃逸防护）——Claude Code 权限与沙箱思路的极简版。
- **apply_patch 补丁格式**：`apply_patch` 采用一段文本补丁（Begin/End 包裹的 Add/Delete/Update/Move hunk），update hunk 用 `@@` 上下文锚点 + ` `/`-`/`+` 行前缀定位改动，支持多块编辑与逐级放宽的模糊匹配。

### 终端界面

- **TUI**：`internal/tui` 基于 Bubble Tea（`bubbles/textarea` 输入、`bubbles/viewport` 式自绘滚动、`lipgloss` 配色）实现**全屏（alternate screen）**交互：启动字标、可滚动对话区（含滚动条）、多行输入框（含命令补全）、状态行。助手 markdown 渲染（含 chroma 代码高亮）、审批弹窗、slash 命令、会话选择器均已纳入。

### 已知限制

- **不做进程沙箱**：安全靠「审批 + 工作目录约束」——有意取舍（见「跨平台」）。
- **`shell` 工具为 Unix-only**（`/bin/bash`），Windows 需适配。
- `shift+enter` 换行不可用（Bubble Tea v1 无增强键盘协议），用 `Ctrl+J` / `Alt+Enter`。
- 全屏 TUI 退出后不保留终端 scrollback（alternate screen 的固有代价）。

## 目录结构

```text
Equus/
├── cmd/equus/               # 进程入口
├── configs/                 # 配置示例
├── docs/                    # 路线图与 git 命令备忘
├── internal/
│   ├── cli/                 # Cobra：根命令(chat) / exec / sessions / config / version
│   ├── config/              # YAML 加载、env 展开、校验、脱敏
│   ├── agent/               # Agent 循环、内置工具、子代理、审批
│   │   └── patch/           # apply_patch 补丁解析与应用
│   ├── approvals/           # 持久化的审批例外（approvals.yaml）
│   ├── mcp/                 # MCP（stdio）客户端
│   ├── skill/               # SKILL.md 技能发现与解析
│   ├── threadstore/         # 会话 JSONL 持久化
│   ├── tui/                 # Bubble Tea 全屏交互界面
│   └── llm/                 # Provider 中立模型域
│       ├── openai/          # Chat Completions / Responses 适配器
│       └── anthropic/       # Anthropic Messages 适配器
├── Makefile
└── go.mod
```

## 构建与运行

需要 Go 1.25 或更高版本。

```bash
make build    # 产物: bin/equus
make check    # fmt + vet + test + build
```

### 跨平台

Equus 是**纯 Go 实现，不依赖 CGO 与任何平台专属系统调用**，因此可直接交叉编译到三大平台：

```bash
GOOS=linux   GOARCH=amd64 go build -o equus-linux    ./cmd/equus
GOOS=darwin  GOARCH=arm64 go build -o equus-darwin   ./cmd/equus
GOOS=windows GOARCH=amd64 go build -o equus.exe      ./cmd/equus
```

配置、LLM 客户端（三种 wire）、Agent 循环、文件工具、TUI、会话持久化等**核心能力均跨平台可用**。

> **唯一的平台例外**：`shell` 工具目前调用 `/bin/bash -lc`，仅适用于 Linux/macOS；Windows 需另行适配（改用 `cmd`/`powershell`）。除此之外无平台专属代码。
>
> Equus **不做进程沙箱**（seccomp / landlock / 容器等）——那会引入平台专属内核能力、破坏这里的一致性。安全靠「审批 + 工作目录约束」，属有意的取舍。

### 配置

首次运行会在 `~/.equus/`（或 `$EQUUS_HOME`）生成初始配置。也可以手动放置：

```bash
mkdir -p ~/.equus
cp configs/config.yaml.example ~/.equus/config.yaml
```

#### API Key 的三种注入方式

**方式一：密钥文件（推荐）** —— 密钥单独存放，配置文件里不出现明文：

```bash
echo "sk-..." > ~/.equus/api_key.txt
chmod 600 ~/.equus/api_key.txt
```

```yaml
model_providers:
  openai:
    wire_api: chat_completions
    api_key_file: api_key.txt   # 相对路径基于 ~/.equus/，也可用绝对路径
    base_url: https://llmapi.isrc.ac.cn/v1
```

**方式二：环境变量** —— 适合临时切换或 CI 环境：

```bash
export EQUUS_API_KEY="sk-..."
```

**方式三：配置文件内联**（配合 `${VAR}` 展开可避免明文落盘）：

```yaml
model_providers:
  openai:
    api_key: ${EQUUS_API_KEY}   # 或直接写明文（不推荐）
```

优先级：**环境变量 > 配置文件内联（`api_key`）> 密钥文件（`api_key_file`）**。文件内容会自动去除首尾空白（换行符安全）。

> **注意：`api_key` 与 `api_key_file` 可以同时存在，但 `api_key` 优先。**
> 只要 `api_key` 非空，就会直接使用它并**忽略 `api_key_file`**（文件甚至不会被读取，不存在也不报错）。
> 常见误配：想用密钥文件却在 `api_key` 里写了**文件路径**（如 `api_key: api_key.txt`）——这会把路径字符串当成密钥发出去，导致 `401 Invalid token`。二者选其一即可；用密钥文件时请只写 `api_key_file`。

其他常用环境变量：

```bash
export EQUUS_BASE_URL="https://llmapi.isrc.ac.cn/v1"
export EQUUS_MODEL="GLM-5.3"
```

| 环境变量 | 作用 |
|---|---|
| `EQUUS_HOME` | 配置目录（默认 `~/.equus`） |
| `EQUUS_API_KEY` | 覆盖所选 Provider 的 api_key |
| `EQUUS_BASE_URL` | 覆盖所选 Provider 的 base_url |
| `EQUUS_MODEL` | 覆盖模型名 |
| `EQUUS_MODEL_PROVIDER` | 选择 model_providers 中的别名 |

### 使用

```bash
# 交互式 TUI 对话（直接运行，需要真实终端）
equus

# 交互式选择器：挑选历史会话恢复（类似新建则不带 --resume）
equus --resume

# 继续最近一次会话 / 按 id 恢复
equus --continue
equus --resume <session-id>

# 列出已保存的会话
equus sessions

# 一次性执行（不保存会话）
equus exec "读取 note.txt 并总结"

# 检查 / 查看配置（密钥已脱敏）
equus config check
equus config show
```

会话以 append-only JSONL 保存在 `~/.equus/sessions/`（可用 `EQUUS_HOME` 覆盖）。**直接运行 `equus`** 即启动交互 TUI 并默认保存会话（`--no-save` 关闭）；`exec` 不保存。

`equus` 是一个 Bubble Tea **全屏 TUI**（alternate screen）：上方是自绘的对话区（含工具调用），下方是输入框面板与状态行。输入框支持完整行编辑（退格、左右移动、`↑`/`↓` 在多行内移动光标，到首/末行才召回历史）。`PgUp`/`PgDn` 与滚轮可滚动对话区。

- **命令补全**：输入 `/` 弹出命令列表，`↑/↓` 选择、`Tab` 补全。
- **换行**：`Ctrl+J`（所有终端可用）或 `Alt+Enter`；`Enter` 提交。
- **退出**：`Ctrl-D`（输入为空时）或 `Ctrl-C`（空闲时）；运行中 `Esc`/`Ctrl-C` 打断当前回合。

> 全屏（alternate screen）模式下，退出后本次对话**不保留**在终端 scrollback 中（与 Claude Code 一致）；这是 Bubble Tea 下可靠支持多行输入的必要取舍。

> 交互模式需要 stdin 为终端；在管道/重定向环境（如 `echo ... | equus`）下会报错并退出——非交互场景请用 `equus exec`。

#### MCP 与 Skill（可选）

```yaml
# 连接 stdio MCP server，其工具会以 mcp__<server>__<tool> 注册
mcp:
  servers:
    my-server:
      command: my-mcp-server
      args: ["--flag"]

# 审批策略（每类：ask / allow / deny）
approval:
  policy: on-request
  shell: ask
  write: allow
  network: ask
  external: ask   # MCP 工具

# Markdown 渲染与语法高亮
markdown:
  enabled: true
  syntax_highlight: true

# 上下文自动压缩
compaction:
  enabled: true
  threshold: 0.8
  keep_recent_turns: 4
```

**Skill**：把 `SKILL.md`（YAML frontmatter 含 `name`/`description` + 正文）放到 `~/.equus/skills/<name>/SKILL.md` 或 `<项目>/.equus/skills/<name>/SKILL.md`，启动时会在系统提示词中列出。

## 已验证

- `make check`（gofmt / go vet / go test / go build）全绿；含 `internal/llm`、`internal/cli` 等包的测试。
- GitHub Actions CI（push/PR 跑 `make check`）。
- 真实端点（`https://llmapi.isrc.ac.cn/v1`，DeepSeek-V4.1-Flash / Anthropic Messages 兼容端点）：
  - `equus exec` 单轮问答、工具往返（读 → 改 → 复验）✅
  - `equus`（TUI）多轮记忆、流式输出、行编辑、命令补全、会话选择器 ✅
  - `apply_patch` / `glob` / `grep` / `shell`（含审批弹窗）✅
  - **三种 wire**：OpenAI Chat Completions / Responses / Anthropic Messages ✅
  - **MCP**（stdio）与 **子代理**（spawn/wait）端到端 ✅
  - **web_fetch**（真实抓取公开页面）✅
  - **Skill** 发现与按需读取 ✅
