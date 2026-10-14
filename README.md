# Equus

Equus 是一个主要仿照 Codex（OpenAI 终端 Coding Agent）架构搭建的极简终端 Coding Agent，使用 Go 编写。目前实现了「配置加载 → LLM 客户端 → Agent 循环 → 工具执行 → CLI」这条主链，交互模式为基于 Bubble Tea 的全屏 TUI；其他高级能力（会话持久化、MCP、Skill、审批等）还在实现过程中。文件工具部分参照了 Claude Code。

Equus（马属）—— 一匹小马。

## 为什么用 Go

Codex 用 Rust 写，Claude Code 用 TypeScript 写，Equus 选择 Go，理由很朴素：

- **Rust**：性能和内存安全无可挑剔，但所有权、生命周期这套编译期约束学习成本高，改一处代码常常要连锁改十处，不适合快速迭代一个还在成长中的框架。
- **TypeScript**：生态最丰富、上手最快，但产物依赖 Node 运行时，发给别人不是「一个文件」；调试 agent 主循环时层层异步回调的调用栈也不够直观。
- **Go**：`go build` 直接产出一个静态链接的单文件二进制，拷到任意 Linux 机器就能跑；并发就是一个 `go` 关键字；标准库自带 HTTP/JSON，核心依赖精简（LLM 域仅 cobra + yaml，TUI 引入 charmbracelet 系列）；类型系统「够用就好」，代码读起来接近直白的英文，重构很快。

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
  - `shell` —— 在工作目录执行 shell 命令（默认需审批）。
- **审批策略**：高危操作（默认 `shell`）按配置决定是询问 / 放行 / 拒绝；交互模式弹窗确认（`[y]` 一次 / `[a]` 本会话 / `[n]` 拒绝），非交互模式默认拒绝。
- **自动压缩（compaction）**：上下文接近窗口上限（默认 80%）时，自动把较早历史摘要成一段、保留最近若干轮原文；状态行显示 `ctx N%`。
- **系统提示词**：自动注入工作目录、平台与工具使用引导；若项目内含 `AGENTS.md`（项目根到工作目录逐层），其内容会一并注入并声明优先。可在配置中用 `base_instructions` 完全覆盖。
- YAML 配置 + 环境变量覆盖（`${VAR}` 展开与 `EQUUS_*` 变量）。
- API Key 支持三种注入方式：配置文件内联 / 密钥文件（`api_key_file`）/ 环境变量。
- 交互式 TUI（Bubble Tea）：全屏对话、行编辑输入框、历史召回、滚动 transcript、实时状态。

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

- **TUI**：`internal/tui` 基于 Bubble Tea（`bubbles/textarea` 输入、`lipgloss` 配色）实现**全屏（alternate screen）**交互：自绘可滚动对话区 + 输入框面板 + 状态行。仅实现 Equus 当前能力范围，markdown 渲染、审批对话框、slash command、session 选择器等尚未纳入。

### 待实现的能力

会话持久化（thread/rollout）、MCP、Skill、审批（approval）、compaction、multi-agent、web fetch/search 等还在实现过程中。

## 目录结构

```text
Equus/
├── cmd/equus/               # 进程入口
├── configs/                 # 配置示例
├── internal/
│   ├── cli/                 # Cobra 命令：chat / exec / config / version
│   ├── config/              # YAML 加载、env 展开、校验、脱敏
│   ├── agent/               # Agent 循环与内置工具
│   │   └── patch/           # apply_patch 补丁解析与应用
│   ├── tui/                 # Bubble Tea 全屏交互界面
│   └── llm/                 # Provider 中立模型域
│       └── openai/          # Chat Completions 适配器（非流式 + SSE 流式）
├── Makefile
└── go.mod
```

## 构建与运行

需要 Go 1.24 或更高版本。

```bash
make build    # 产物: bin/equus
make check    # fmt + vet + test + build
```

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
# 交互式 TUI 对话（需要真实终端）
equus chat

# 继续最近一次会话 / 按 id 恢复
equus chat --continue
equus chat --resume <session-id>

# 列出已保存的会话
equus sessions

# 一次性执行（不保存会话）
equus exec "读取 note.txt 并总结"

# 检查 / 查看配置（密钥已脱敏）
equus config check
equus config show
```

会话以 append-only JSONL 保存在 `~/.equus/sessions/`（可用 `EQUUS_HOME` 覆盖）。`equus chat` 默认保存（`--no-save` 关闭）；`exec` 不保存。

`chat` 是一个 Bubble Tea **全屏 TUI**（alternate screen）：上方是自绘的对话区（含工具调用），下方是输入框面板与状态行。输入框支持完整行编辑（退格、左右移动、`↑`/`↓` 在多行内移动光标，到首/末行才召回历史）。`PgUp`/`PgDn` 与滚轮可滚动对话区。

- **换行**：`Ctrl+J`（所有终端可用）或 `Alt+Enter`；`Enter` 提交。
- **退出**：`Ctrl-D`（输入为空时）或 `Ctrl-C`（空闲时）；运行中 `Esc`/`Ctrl-C` 打断当前回合。

> 全屏（alternate screen）模式下，退出后本次对话**不保留**在终端 scrollback 中（与 Claude Code 一致）；这是 Bubble Tea 下可靠支持多行输入的必要取舍。

> `chat` 需要 stdin 为交互式终端；在管道/重定向环境（如 `echo ... | equus chat`）下会报错并退出——非交互场景请用 `equus exec`。

## 已验证

- `make check`（fmt/vet/test/build）全绿。
- 真实端点（`https://llmapi.isrc.ac.cn/v1`，GLM-5.3 / DeepSeek-V4-Flash）：
  - `equus exec` 单轮问答 ✅
  - `equus chat`（TUI）多轮记忆（"我叫小明" → "我叫什么名字？"）✅
  - `equus chat`（TUI）流式对话：输入、退格删除、`Ctrl-D` 退出均正常 ✅
  - 工具往返：读取 `note.txt` → 写入 `answer.txt` ✅
  - 流式 reasoning / 内容 / token 用量输出 ✅
