# 各模块 git 命令（交用户执行）

> 本仓库约定：**所有 git 操作由用户执行**，本文件只记录建议的 `git add` / `git commit` 命令。
> `change.log` 被 `.gitignore` 忽略（`*.log`），不纳入提交。

## 提交状态

### 已完成（roadmap A~F + TUI）

```
adc46f5 feat(llm): add Anthropic Messages and OpenAI Responses adapters          # F
27bbdc0 feat(agent): environment-aware system prompt and AGENTS.md support       # E
19382e3 feat(agent): auto-compact context and report context usage               # D
2b0476b feat(session): persist conversations and add resume/continue             # C
83bb0ac feat(agent): add shell tool with configurable approval policy            # B
73d35fc feat(agent): add apply_patch, glob and grep tools                        # A
ab92d80 feat(tui): add Bubble Tea inline chat (replaces REPL)
1cd87da feat(tui): add Bubble Tea interactive chat (replaces REPL)
c0bc4fa fix(agent): don't fall back to blocking on terminal provider errors
4b557d7 feat(config): support api_key_file as an API key source
6ac602b feat(build): install equus to ~/.local/bin on make build
```

### 待提交

| 改动 | 内容 | 命令 |
|---|---|---|
| 18 | TUI 启动 logo + 输入框修复 | 见下 |
| 19 | Markdown 渲染 + chroma | 见下 |

```bash
# 改动 18 + 19（均在 internal/tui，且改动了 README 与 go.mod）
git add internal/tui README.md go.mod go.sum
git commit -m "feat(tui): startup logo, input fixes, and markdown rendering"

# docs（roadmap.md 等）
git add docs
git commit -m "docs: rework roadmap (front-end focus) and git notes"
```

---

## 延伸方向 G~S：各模块命令

### G. Markdown 渲染 + chroma（改动 19）
```bash
git add internal/tui/markdown.go internal/tui/markdown_table.go internal/tui/markdown_test.go \
        internal/tui/render.go internal/tui/view.go internal/tui/model.go internal/tui/run.go \
        internal/config/markdown.go internal/config/config.go internal/config/patch.go \
        internal/cli/chat.go go.mod go.sum README.md
git commit -m "feat(tui): markdown rendering with chroma syntax highlighting"
```

### H. Slash 命令（改动 20）
```bash
git add internal/tui/slash.go internal/tui/slash_dispatch.go internal/tui/slash_test.go \
        internal/tui/update.go internal/agent/session.go
git commit -m "feat(tui): slash commands (/help, /clear, /compact, /status, ...)"
```

### I. 输入框命令补全（改动 24）
```bash
git add internal/tui/slash.go internal/tui/slash_popup.go internal/tui/slash_popup_test.go \
        internal/tui/update.go internal/tui/view.go internal/tui/model.go README.md
git commit -m "feat(tui): slash-command completion popup"
```

### P. 审批策略增强（改动 21）
```bash
git add internal/approvals internal/config/approval.go internal/config/patch.go internal/config/validation.go \
        internal/agent/approval.go internal/agent/session.go internal/agent/shell_test.go \
        internal/cli/chat.go internal/cli/exec.go internal/tui/update.go internal/tui/view.go \
        internal/config/loader_test.go README.md
git commit -m "feat(agent): shell allowlist, --allow-shell, and persistent approvals"
```

### J. 会话选择器（改动 23）
```bash
git add internal/tui/session_picker.go internal/tui/session_picker_test.go \
        internal/tui/model.go internal/tui/update.go internal/tui/view.go internal/tui/run.go \
        internal/tui/slash.go internal/tui/slash_dispatch.go \
        internal/threadstore/store.go internal/threadstore/store_test.go \
        internal/cli/cli.go internal/cli/chat.go README.md
git commit -m "feat(tui): session picker, rename/delete, and root command runs chat"
```

### S. 测试与 CI（改动 22）
```bash
git add internal/llm/llm_test.go internal/cli/cli_internal_test.go .github/workflows/ci.yml README.md
git commit -m "test: cover llm and cli packages; add GitHub Actions CI"
```

### K. 更丰富的状态栏（改动 25）
```bash
git add internal/tui/git.go internal/tui/git_test.go internal/tui/status_line.go internal/tui/model.go internal/tui/run.go README.md
git commit -m "feat(tui): show git branch in the status line"
```

### L. 自绘滚动条（改动 27）
```bash
git add internal/tui/scrollbar.go internal/tui/scrollbar_test.go internal/tui/view.go README.md
git commit -m "feat(tui): transcript scrollbar"
```

### M. MCP 接入（改动 30）
```bash
git add internal/mcp internal/agent/mcp_tool.go internal/agent/mcp_tool_test.go internal/agent/approval.go \
        internal/config/mcp.go internal/config/config.go internal/config/patch.go internal/config/validation.go \
        internal/cli/mcp.go internal/cli/chat.go internal/cli/exec.go \
        go.mod go.sum README.md .github/workflows/ci.yml
git commit -m "feat(mcp): connect stdio MCP servers and register their tools"
```

### N. Skill 加载（改动 29）
```bash
git add internal/skill internal/agent/instructions.go internal/agent/instructions_test.go internal/cli/chat.go
git commit -m "feat(skill): discover SKILL.md skills and advertise them in the prompt"
```

### O. multi-agent 子代理（改动 31）
```bash
git add internal/agent/subagent.go internal/agent/subagent_test.go internal/agent/builtin.go \
        internal/agent/session.go internal/cli/chat.go
git commit -m "feat(agent): async read-only subagents (spawn/wait/close)"
```

### Q. web_fetch 工具（改动 28）
```bash
git add internal/agent/webfetch.go internal/agent/webfetch_test.go internal/agent/approval.go \
        internal/agent/builtin.go internal/config/approval.go internal/config/patch.go internal/config/validation.go
git commit -m "feat(agent): add SSRF-guarded web_fetch tool"
```

### R. token 用量累计统计（改动 26）
```bash
git add internal/agent/session.go internal/agent/session_test.go \
        internal/tui/model.go internal/tui/update.go internal/tui/slash_dispatch.go internal/tui/slash_test.go
git commit -m "feat(agent): cumulative token usage in /status"
```


> 每项落地后，在此补上对应的 `git add` / `git commit` 命令。
