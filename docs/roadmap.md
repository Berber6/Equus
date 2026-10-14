# Equus 开发路线图（Roadmap）

本文件记录 Equus 的增量开发计划，按**优先级**排列。每一项是一个可独立交付的增量，包含目标、缺口、范围、风险与验证方式。

状态图例：✅ 已完成　🚧 进行中　⬜ 待开始

---

## 总览

| 代号 | 任务 | 状态 | 优先级 | 规模 | 依赖 |
|---|---|---|---|---|---|
| A | 文件工具三件套（apply_patch / glob / grep） | ✅ 已完成 | 高 | 中 | 无 |
| B | shell 工具 + 审批 | ✅ 已完成 | 高 | 中 | 无 |
| C | 会话持久化 + resume | ✅ 已完成 | 中 | 中 | 无 |
| D | compaction / 上下文窗口管理 | ✅ 已完成 | 中 | 中-高 | 依赖 C（可选） |
| E | 系统提示词增强 + AGENTS.md | ✅ 已完成 | 中 | 低 | 无 |
| F | 多 Provider 适配（Messages / Responses） | ✅ 已完成 | 低 | 中 | 无 |

**延伸方向（G~S）全部完成：**

| 组 | 代号 | 任务 | 状态 |
|---|---|---|---|
| 前端 | G | Markdown 渲染 + chroma | ✅ 已完成 |
| 前端 | H | Slash 命令 | ✅ 已完成 |
| 前端 | I | 输入框命令补全 | ✅ 已完成 |
| 前端 | J | 会话选择器 | ✅ 已完成 |
| 前端 | K | 状态栏 git 分支 | ✅ 已完成 |
| 前端 | L | 自绘滚动条 | ✅ 已完成 |
| 能力 | M | MCP 接入（stdio） | ✅ 已完成 |
| 能力 | N | Skill 技能加载 | ✅ 已完成 |
| 能力 | O | multi-agent / 子代理 | ✅ 已完成 |
| 能力 | P | 审批策略增强 | ✅ 已完成 |
| 能力 | Q | web_fetch 工具 | ✅ 已完成 |
| 工程 | R | token 用量累计统计 | ✅ 已完成 |
| 工程 | S | 测试与 CI | ✅ 已完成 |

**明确不做**：进程沙箱、`shift+enter` 换行（见文末）。

---

## A. 文件工具三件套（apply_patch / glob / grep）✅

**目标**：让模型能改代码、能找代码，而非只能整文件读写。

**已完成**：
- `apply_patch`：一段文本补丁（`*** Begin Patch` … `*** End Patch`），支持 Add / Delete / Update / Move、多块编辑、`@@` 上下文锚点、`*** End of File`、四级模糊定位（精确 → 去尾空白 → 去首尾空白 → Unicode 标点归一化）。实现在 `internal/agent/patch/`。
- `glob`：`**` 递归匹配（自实现，无新依赖），跳过噪声目录，结果排序，上限 200。
- `grep`：Go 正则按行搜索，输出 `path:line: text`，支持 glob 限定 / 大小写 / 跳过二进制，上限 200。

**安全**：全部经 `resolvePath` 做工作目录逃逸防护。

**验证**：`make check` 全绿 + 单测 + 真实端点端到端（grep→read→apply_patch→grep 闭环）。

---

## B. shell 工具 + 最小审批 ✅

**目标**：让 agent 能跑测试/构建/命令，形成"改 → 验"闭环。当前 Equus 能读、能找、能改，唯独不能**验证**自己的改动（跑不了 `go test` / `go build`）。

**缺口**：
- 无任何命令执行能力。
- 写文件 / 执行命令无任何确认——这是**危险面最大**的能力，必须与审批捆绑。

**范围**：
1. `shell` 工具：入参 `{command, timeout?}`，在工作目录内执行，返回 stdout/stderr/退出码（含截断）。
2. **最小审批机制**：
   - agent 发起需审批的操作（shell 命令、写文件、apply_patch）时，向 UI 发审批事件并阻塞等待决定；
   - 审批结果：允许一次 / 本次会话允许 / 拒绝；
   - TUI 里弹出确认（←/→ 选择、回车确认）；
   - `exec`（非交互）模式：默认拒绝危险操作或按配置放行。
3. 配置项：审批策略（如 `approval_policy: on-request / never`）、命令白/黑名单（可选）。

**关键设计点**：
- 审批是**跨层**能力：agent 层产生审批事件 → cli/tui 层决策 → 回填 agent。需要给 `agent.Event` 增加审批种类，或引入独立的审批回调（`session.Options` 里加 `Approval func(...) Decision`）。
- 参考 Claude Code 的 `Permission` / Codex 的 `AskForApproval` 思路，但保持极简：先做"允许一次 / 拒绝"两档，后续再加"会话级/永久"。
- 超时与 deny-by-default：非交互环境无确认渠道时默认拒绝。

**风险**：高（任意命令执行）。**缓解**：审批 + 工作目录内 + 超时 + 输出截断。

**验证**：端到端让模型"改一个文件 → 跑 `go test`/`go build` 验证 → 报告结果"；审批弹窗在 TUI 中可用；非交互模式拒绝危险命令。

---

## C. 会话持久化 + resume（thread/rollout）✅

**目标**：解决"关掉就没了"。当前会话仅存于进程内存（`Session.history`）。

**缺口**：无落盘、无恢复、无历史列表。

**范围**：
1. **rollout 事件日志**：把每轮对话（用户消息、助手消息、工具调用与结果、token 用量）以 **JSONL** 追加写入 `~/.equus/sessions/<id>.jsonl`。
2. **thread 元数据**：会话 id、创建时间、模型、工作目录、标题（首条用户消息摘要）。
3. **CLI**：
   - `equus chat --resume [id]` / `--continue`（续最近一次）；
   - `equus sessions` 列出历史会话。
4. 反序列化 `llm.ResponseItem` 重建 `Session.history`。

**关键设计点**：
- 事件日志（append-only）而非整文件快照——抗崩溃、易增量。
- 序列化边界：`llm.ResponseItem` / `ToolCall` 已可 JSON 化，复用其 tag。
- 与 `exec` 的关系：`exec` 是否也落盘？建议默认否（一次性语义），可加 `--save`。

**验证**：对话 → 退出 → `--resume` 恢复上下文继续对话；崩溃后日志不损坏。

---

## D. compaction / 上下文窗口管理 ✅

**目标**：长会话不超上下文窗口。

**缺口**：`history` 无限增长，超出模型窗口后请求会失败。

**范围**：
1. **token 计数**：估算当前 history 的 token 占用（粗略：字符数/4，或用 usage 回填的真实值）。
2. **触发阈值**：接近窗口上限（如 80%）时触发 compaction。
3. **compaction 策略**：把较早的 history 摘要成一段系统/助手消息，保留最近 N 轮原文；或分层裁剪工具结果。
4. 与持久化配合：compaction 作为 rollout 里的事件（可追溯）。

**关键设计点**：
- 优先用真实 usage（OpenAI 返回的 prompt_tokens）而非估算。
- 摘要本身是一次模型调用——需防"摘要也超窗口"。
- 可参考 Codex 的 context manager / Claude Code 的 compact 思路。

**验证**：构造超长会话，确认自动 compaction 后请求仍成功且指代基本不丢。

---

## E. 系统提示词增强 + AGENTS.md ✅

**目标**：当前 base instructions 是一句硬编码（"You are Equus…"）。补上工作目录感知与工具使用引导，提升工具调用质量。

**缺口**：模型不知道 cwd、不知道有哪些工具的正确用法、缺少"改代码要用 apply_patch 而非 write_file"这类引导。

**范围**：
1. base instructions 注入：工作目录、平台、可用的工具清单与使用准则。
2. 引导要点：查找用 glob/grep、改动用 apply_patch（带上下文）、大文件先 read 再改。
3. 可配置：允许通过配置文件/环境变量覆盖系统提示词。

**规模**：低。**验证**：观察模型工具选择是否更合理（少用 write_file 整写、多用 apply_patch）。

---

## F. 多 Provider 适配（Anthropic Messages + Responses）✅

**目标**：当前只支持 OpenAI Chat Completions。补 Anthropic Messages 与 Responses API 两种 wire 格式。

**缺口**：`config.WireAPI` 只有 `chat_completions`；`buildSession` 对其它值直接报错。

**范围**：
1. `internal/llm/anthropic`：Anthropic Messages API 适配器（`/v1/messages`，事件流 SSE）。
2. `internal/llm/openai`：补 Responses API 方言（可选）。
3. `wire_api` 配置扩展 + `buildSession` 按 wire_api 选择客户端。

**关键设计点**：
- `llm` 中立域已就绪，适配器只做传输转换——这正是分层设计的回报。
- 各家的 tool-call 表示不同（Anthropic 的 `tool_use`/`tool_result` block vs OpenAI 的 `tool_calls`），在中立域到 wire 的转换里吸收。

**验证**：切换 wire_api 后同一套 agent 循环可跑通对话与工具调用。

---

## 推进顺序（A~F 均已完成）

1. **A** ✅ 文件工具 —— 打底能力。
2. **B** ✅ shell + 审批 —— 形成"改→验"闭环。
3. **C** ✅ 持久化 + resume —— 解决易用性痛点。
4. **E** ✅ 提示词 + AGENTS.md —— 提升工具使用质量。
5. **D** ✅ compaction —— 长会话必需。
6. **F** ✅ 多 Provider —— 扩展兼容面。

> 说明：每个增量都应同步更新 `change.log`（本项目约定），并保持 `make check` 全绿。

---

# 延伸方向（A~F 之外的后续）

A~F 已完成 Equus 的核心骨架。以下是可继续推进的延伸项，**按前端（TUI）→ 能力 → 工程**分组并排序。

## 前端（TUI）

> 这一组是本路线图的**当前重点**：主链与安全已就绪，接下来集中打磨交互界面。

### G. Markdown 渲染（含语法高亮） ✅
- **目标**：TUI 里把助手的 markdown（标题、列表、代码块、行内代码、表格）渲染成带样式的文本，代码块用 **chroma 语法高亮**。
- **缺口**：当前 `assistantEntry` 只做纯文本 + 换行。
- **要点**：
  - 引入 `github.com/alecthomas/chroma/v2`（注释：**新增依赖**，与"尽量精简"取舍，因渲染质量提升明显）。
  - markdown 解析 → 带语义样式的行；代码围栏交给 chroma 按语言高亮，并按终端色深选择 formatter（`terminal16m`/`terminal256`/`terminal16`）。
  - 无彩色/`NO_COLOR` 时退化为纯文本。

### H. Slash 命令 ✅
- **目标**：`/help`、`/clear`、`/compact`、`/model`、`/resume` 等交互命令。
- **要点**：命令解析 + 注册表 + TUI 内联提示/弹窗补全；参考实现有一套命令体系（含参数内联）。

### I. 输入框内联高亮 / 补全 ✅
- **目标**：输入框内对 slash 命令、文件路径（`@` 引用）做高亮与补全。
- **依赖**：H（slash 命令）先落地。

### J. 会话选择器 ✅
- **目标**：`chat --resume` 无参数时弹出**会话选择器**（列表 + 搜索 + 预览）；`/rename`、`/delete`；会话标题自动生成。
- **现状**：C 已实现按 id 恢复 + `equus sessions`；缺交互式选择与管理。

### K. 更丰富的状态栏 ✅
- **目标**：状态栏显示 git 分支、模型 reasoning effort、token 预算等。
- **缺口**：需要 git 集成与更多模型元数据。

### L. 自绘滚动条 + 鼠标选择 ✅
- **目标**：对话区右侧滚动条；可选鼠标选中复制。
- **现状**：已支持滚轮滚动（`WithMouseCellMotion`），无滚动条与选择。

## 能力扩展

### M. MCP（Model Context Protocol） ✅
- **目标**：接入 MCP server，把其工具动态注册进 agent 工具集。
- **要点**：新增 `internal/mcp`（连接、握手、工具发现与调用桥接）；工具注册表支持运行时增删。

### N. Skill（可加载技能） ✅
- **目标**：把可复用的指令/流程做成"技能"，按需注入提示词或触发。
- **要点**：技能发现（目录约定）+ 提示词注入 + TUI 展示。

### O. multi-agent / 子代理 ✅
- **目标**：主 agent 派出子代理执行子任务（各自上下文），汇总结果。
- **要点**：子会话隔离、工具白名单、结果回填；参考实现有完整生命周期。
- **前置**：审批的"分层信任"（见 P）——子代理不宜逐个弹窗。

### P. 审批策略增强（subagent 前置） ✅
- **目标**：命令级**前缀白名单**自动放行、`exec` 非交互的放行标志、审批**持久化允许**（跨会话）。
- **现状**：B 已实现 ask/allow/deny + **会话级**允许；缺更细粒度规则与持久化。
- **意义**：这是 future multi-agent 的必要前置——不能要求用户对每个子代理的每条命令逐一审批。

### Q. web fetch / search ✅
- **目标**：`web_fetch`（抓取 URL）、`web_search`（搜索）工具。
- **要点**：新增工具 + 网络请求 + 输出截断；需注意审批/安全（外部请求）。

## 工程

### R. 响应缓存 / 成本统计 ✅
- **目标**：跨轮次 token 成本累计；prompt caching（若端点支持）。
- **现状**：usage 已采集（含 streaming）；未做累计与缓存。

### S. 测试与 CI ✅
- **目标**：补齐 `internal/cli` 与 `internal/llm` 的测试；加 CI（fmt/vet/test/build）。
- **现状**：`cli`、`llm` 顶层包无测试文件。

---

## 完成情况

A~S **全部已完成**。推进次序：A → B → C → D → E → F → G → H → I → J → K → L → M → N → O → P → Q → R → S。

> 每项的落地细节见 `change.log`；对应 commit 命令见 `docs/git-commands.md`。

## 明确不做

- **沙箱执行**：seccomp/landlock/容器等平台专属内核能力会破坏跨平台一致性；安全改由「审批 + 工作目录约束 + 前缀白名单」承担。
- **`shift+enter` 换行**：Bubble Tea v1 不支持终端增强键盘协议（Kitty），`shift+enter` 与 `enter` 字节无法区分；维持 `ctrl+j` / `alt+enter`。

> 每项落地时：新增 `internal/<pkg>`（如适用）、同步更新 `README.md` 与 `change.log`、保持 `make check` 全绿、并在 `docs/git-commands.md` 记录对应 commit 命令。
