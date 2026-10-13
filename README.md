# Equus

Equus 是一个主要仿照 Codex（OpenAI 终端 Coding Agent）架构搭建的极简终端 Coding Agent，使用 Go 编写。目前实现了「配置加载 → LLM 客户端 → Agent 循环 → 工具执行 → CLI」这条主链，其他高级能力（TUI、会话持久化、MCP、Skill、审批等）还在实现过程中。文件工具部分参照了 Claude Code。

Equus（马属）—— 一匹小马。

## 为什么用 Go

Codex 用 Rust 写，Claude Code 用 TypeScript 写，Equus 选择 Go，理由很朴素：

- **Rust**：性能和内存安全无可挑剔，但所有权、生命周期这套编译期约束学习成本高，改一处代码常常要连锁改十处，不适合快速迭代一个还在成长中的框架。
- **TypeScript**：生态最丰富、上手最快，但产物依赖 Node 运行时，发给别人不是「一个文件」；调试 agent 主循环时层层异步回调的调用栈也不够直观。
- **Go**：`go build` 直接产出一个静态链接的单文件二进制，拷到任意 Linux 机器就能跑；并发就是一个 `go` 关键字；标准库自带 HTTP/JSON，本项目核心依赖只有 2 个；类型系统「够用就好」，代码读起来接近直白的英文，重构很快。

一句话：在「够快的性能」和「够快的开发速度」之间取中庸，Go 最省心。

## 功能

- 与任意 OpenAI Chat Completions 兼容端点对话（OpenAI、GLM、DeepSeek、Qwen 等）。
- SSE 流式输出，实时显示思考过程（reasoning）、工具调用与 token 用量。
- 多轮对话记忆。
- 内置文件工具：`read_file`、`write_file`、`list_files`（限制在工作目录内）。
- YAML 配置 + 环境变量覆盖（`${VAR}` 展开与 `EQUUS_*` 变量）。
- API Key 支持三种注入方式：配置文件内联 / 密钥文件（`api_key_file`）/ 环境变量。

## 设计来源

### 主要仿照 Codex

- **分层架构与依赖方向**：`cmd` 进程入口 → `cli` 命令层 → `config` 配置 → `llm` Provider 中立模型域 → `llm/openai` 传输适配器 → `agent` 主循环。依赖单向、领域与传输分离，与 Codex 的 package 组织方式一致。
- **Provider 中立模型域**：`Request`/`Response`/`ResponseItem`/`ToolCall`/`Client`/`Stream` 等中立类型，模型选择与传输配置解耦（`model_providers` + `wire_api` 抽象）。
- **Agent 主循环**：采样 → 解析工具调用 → 执行 → 工具结果回填 → 继续采样直到 `finish_reason=stop`；流式优先、失败自动回退非流式。
- **Chat Completions SSE 流式**：逐分片解析，包括跨分片的 tool call 参数拼接与 usage 汇总。
- **配置体系**：YAML + `${VAR}` 环境引用展开 + `EQUUS_*` 环境变量覆盖 + 校验与密钥脱敏；密钥可内联、走密钥文件（`api_key_file`）或环境变量。

### 参照了 Claude Code

- **工具契约**：每个工具自带名称、自然语言描述和 JSON Schema 参数定义，由模型自主决定何时调用。
- **文件工具集与安全默认值**：`read_file` / `write_file` / `list_files` 三个文件工具，所有路径限制在工作目录内（`resolvePath` 逃逸防护）——Claude Code 权限与沙箱思路的极简版。

### 待实现的能力

TUI、会话持久化（thread/rollout）、MCP、Skill、审批（approval）、compaction、multi-agent、web fetch/search 等还在实现过程中。

## 目录结构

```text
Equus/
├── cmd/equus/               # 进程入口
├── configs/                 # 配置示例
├── internal/
│   ├── cli/                 # Cobra 命令：chat / exec / config / version
│   ├── config/              # YAML 加载、env 展开、校验、脱敏
│   ├── agent/               # Agent 循环与内置工具
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

优先级：**环境变量 > 配置文件内联 > 密钥文件**。文件内容会自动去除首尾空白（换行符安全）。

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
# 交互式对话
equus chat

# 一次性执行
equus exec "读取 note.txt 并总结"

# 检查 / 查看配置（密钥已脱敏）
equus config check
equus config show
```

`chat` 模式中输入 `\quit` 或 Ctrl-D 退出。

## 已验证

- `make check`（fmt/vet/test/build）全绿。
- 真实端点（`https://llmapi.isrc.ac.cn/v1`，GLM-5.3 / DeepSeek-V4-Flash）：
  - `equus exec` 单轮问答 ✅
  - `equus chat` 多轮记忆（"我叫小明" → "我叫什么名字？"）✅
  - 工具往返：读取 `note.txt` → 写入 `answer.txt` ✅
  - 流式 reasoning / 内容 / token 用量输出 ✅
