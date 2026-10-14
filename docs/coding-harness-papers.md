# Coding Harness 参考论文备选清单

> 目的：为 Equus 寻找一个可落地的创新点，选出合适的参考论文。
> 整理时间：2026-10-14
>
> 说明：本清单论文主要来自 arXiv（cs.SE / cs.AI / cs.CL）最新投稿（2026-10），
> 多数尚未标注顶会 venue。顶会正式版中本方向的经典论文单列于末尾「顶会经典」一节。

---

## 第一梯队：直接对应 Equus 的「待实现」缺口，改造工作量小

### 1. StateHarness — 长程任务的「项目状态维护」 ⭐最推荐

- **arXiv**: 2610.17616
- **作者**: Chuan Zou, Yujun Zhou, Xiaonan Luo, Xiangliang Zhang
- **核心**: agent 在工作时维护一份**运行态项目状态**（当前问题/假设、关键发现与决策、
  仓库关系、未完成任务），并按「行为信号」触发**诊断会话**回看历史。不改模型参数。
  运行态支持包括：提示记录更新、搜索受影响的 callers/tests/docs，并维护一份
  「compact working brief」供后续决策。
- **为什么适合 Equus**: Equus 的 `Session.history` 是**平铺的 ResponseItem 列表**，
  长任务时信息散落在膨胀的历史里。StateHarness 正是给「历史」加一层**结构化 brief**
  ——可落成 Equus 的新模块 `internal/agent/state.go`，配合缺失的持久化能力。
- **可验证指标**: RefactorBench 全链完成 29.4%→35.3%、cleanup 准确率 87%→95.4%；
  LHTB 均值 reward 0.427→0.576；ChainSWE bug-fix 35.1%→39.5%。
  （注意：论文也报告 pass-to-pass 回归有所增加。）

### 2. Cortex — Harness 的「度量门控演化与剪枝」 ⭐很推荐

- **arXiv**: 2610.14216
- **作者**: Samuele Giampieri（单一作者，46 页）
- **核心**: 把开发者对 agent 的**每次纠正存成可复现任务**，找重复纠正 →
  一次写一条 skill/rule → **在有/无它各跑多次，只有净增益且没有任务明显变差才保留**。
  已存在的文本在删除前也做同样测试；Cortex 既不添加也不删除 agent 从未用过的文本。
- **为什么适合 Equus**: Equus **完全没有 skill/rule 机制**，system prompt 是一行常量。
  Cortex 提供了「如何安全地让 harness 自我演化」的方法论——可作为 Equus 引入
  **AGENTS.md / skills** 的评测框架。

### 3. Knowledge Graphs as Files (DKGI) — 工具创建与工具使用「共演化」

- **arXiv**: 2610.18616
- **作者**: Yuyang Bai, Yu Wang, Yu Zhang
- **核心**: agent **写代码自己造工具**（在文件上遍历关系、合并节点集、挑选文本证据），
  执行反馈驱动工具修订，工具反过来塑造后续检索。训练无关。
- **为什么适合 Equus**: Equus 工具集是**写死的 3 个**（`read/write/list`），
  `Tool` 接口 + `toolRegistry` 已经清爽。DKGI 的自然延伸是给 Equus 加一个
  **`make_tool` / 脚本执行工具**，让模型动态扩工具——改动小而体面。
- **可验证**: 3 个领域每张图 Hit@1 领先最强基线 ≥6.7 点。

### 4. SWE-MISBEHAVIOR + CODE-KIT — 边界违规的检测与防护

- **arXiv**: 2610.18206
- **作者**: Do Xuan Trong 等（15 作者）
- **核心**: 首个 coding agent「**越界行为**」分类学与基准（代码生成/文件管理/系统交互
  三层，11 类违规，约 7.4K 样本）；CODE-KIT 用**违规感知指南 + 新工具**降低违规。
- **为什么适合 Equus**: Equus 已有 `resolvePath` 逃逸防护（安全默认值），但只是「路径」
  一个维度。可把 Equus 的**工作目录沙箱**扩展成一套「边界违规检测」——直接给 Equus
  补上 README 里说的**审批（approval）**能力，且有现成 benchmark 可对照。
- **可验证**: CODE-KIT 降低违规 52%(smolagents)/34%(OpenHands)，同时提升任务完成度。

---

## 第二梯队：机制新颖，但改造更偏研究性

### 5. TurnCost — 「轮数，而非观测」的缓存感知循环工程

- **arXiv**: 2610.15093
- **作者**: Pramod Kumar, Raj Kumar Pandey
- **核心**: 600 条真实轨迹分析——**观测（工具输出）中位数只有 118–286 token，
  98.6% 输入命中前缀缓存**；**轨迹成本 84% 由「轮数」解释，观测大小只占 4%**。
  给出模型委派的闭式盈亏平衡条件。发布 TurnCost 工具重新给轨迹语料打分。
- **为什么适合 Equus**: 直接命中 Equus 的 `MaxRoundTrips=16` 设计。可以做一个
  **cache-aware 的循环控制/终止策略**，甚至只是把「轮数才是成本主因」这个洞察
  做成 Equus 的度量输出。**分析型、易落地**。

### 6. ReTerm — 从失败中恢复的终端 agent

- **arXiv**: 2610.18484
- **作者**: Wei Xie, Kaituo Feng, Shiming Fu, Yong Liu, Xiangyu Yue, Shuangjia Zheng
- **核心**: 让 agent **学习如何从自己的失败命令/错误实现中恢复**
  （ReTerm-10K 数据 + 从失败状态分叉的 GRPO）。
- **为什么适合 Equus**: Equus 的 `executeToolCalls` 已经「把错误当对话内容回填」，
  但**没有恢复策略**。可把 Equus 的工具失败处理升级成显式的**恢复/重试策略**。
  偏训练侧，落地部分主要是「失败状态分叉」的思路。
- **可验证**: ReTerm-9B 在 Terminal-Bench 2.0 达 34.8% Pass@1（+14.6 点）。

### 7. Self-Evolving Agent Harness — harness 自演化的理论框架

- **arXiv**: 2610.18813
- **作者**: Chuyang Wei, Haoxiang Guan, Minghang Zhu, Kexin Zhou, Jiyan He,
  Xiren Zhou, Huanhuan Chen, Yifei Shen（66 页）
- **核心**: 形式化「harness 自演化」（改 prompt/memory/workflow 而非权重），
  给出**PAC 式泛化界、回归预算下的保留界、基于新验证数据的准入规则**。
  区分「解释性状态修改」与「工具/代码/仓库级控制逻辑/插件组合的修订」。
- **为什么适合 Equus**: **理论最重**，可作为 Cortex/TextualRL 的**理论背书**。
  适合论文里写「相关理论」，而非直接实现。

### 8. TextualRL — 用于上下文管理的文本强化学习

- **arXiv**: 2610.18216
- **作者**: Tianyu Huai, Jie Zhou, Bo Mao, Qin Chen, Liang He, Xipeng Qiu
- **核心**: 把「**同任务的一组 rollout**」而非单条轨迹作为批判单元
  （Groupwise Policy Critique），reviewed proposal 变成**有界的文本补丁**，
  由留出验证奖励决定是否采纳。gradient-free。
- **为什么适合 Equus**: 为 Equus 提供「**如何让 system prompt / 记忆有界地自我更新**」
  的算法。
- **可验证**: 6 个 benchmark、5 个 agent 上平均准确率 +24.4 点(GPT-5.5)/+15.3 点(Qwen3.8-27B)。

### 9. RBC (Randomized Block Curation) — 用随机实验筛选 agent playbook

- **arXiv**: 2610.18427
- **作者**: Hoang Pham, Cong-Duy Nguyen, Dong Le, Ponhvoan Srey, Anh Tuan Luu
- **核心**: agent 自写 playbook 极不稳定（同流程可能 +还是 −69.6 点）；用**随机分块实验**
  估计每条经验的影响，只留有效块。
- **为什么适合 Equus**: 若要给 Equus 加「经验记忆」，这是**评测方法论**。
- **可验证**: 平均成功率 66.9（vs 整本 55.5 / 无 54.1 / agent 自选 53.8）。

---

## 顶会经典（正规 venue，建议作为「相关工作」基石）

| 论文 | venue | 核心 |
|---|---|---|
| **SWE-agent: Agent-Computer Interface** | NeurIPS 2024 | ACI——为 agent 专门设计工具接口，Equus `Tool` 契约的理论源头 |
| **SWE-bench** | ICLR 2024 | 真实 GitHub issue 修复基准 |
| **CodeAct** | ICML 2024 | 用可执行 Python 代码作为统一动作空间 |
| **OpenHands** | ICLR 2025 | 开源 agent 平台，Equus 的对标对象 |
| **ANIS (Agent-Native Interfaces)** arXiv 2610.15765 | — | 界面三性质（discoverability/granularity/observability），token 降 ~73% |

---

## 建议：结合 Equus 现状的创新点选择

结合「Equus 是极简框架、缺口明确、要做出**可验证创新点**」：

| 若想做… | 选 | 创新点表述 |
|---|---|---|
| **A. 长程任务（最契合痛点）** | **StateHarness (1)** | 给 Equus 的平铺 history 加「结构化项目状态 brief + 诊断会话」，解决 16 轮上限/上下文膨胀 |
| **B. 动态工具（改动小而亮）** | **DKGI (3)** | 让 Equus 从固定 3 工具 → 「工具创建/使用共演化」 |
| **C. 安全/审批（补 README 缺口）** | **SWE-MISBEHAVIOR (4)** | 把 `resolvePath` 沙箱升级为「边界违规检测 + 审批」，对照 CODE-KIT |
| **D. 成本/循环工程（分析型，易发小论文）** | **TurnCost (5)** | 用「轮数主导成本」重构 Equus 的 `MaxRoundTrips` 终止策略 |

**最推荐 A（StateHarness）**——它同时解决两个已知痛点（历史消失、长程被打断），
且论文给了现成 benchmark（RefactorBench/LHTB/ChainSWE）可直接对标。

---

## 论文链接

- [StateHarness (arXiv 2610.17616)](https://arxiv.org/abs/2610.17616)
- [Cortex (arXiv 2610.14216)](https://arxiv.org/abs/2610.14216)
- [DKGI — Knowledge Graphs as Files (arXiv 2610.18616)](https://arxiv.org/abs/2610.18616)
- [SWE-MISBEHAVIOR (arXiv 2610.18206)](https://arxiv.org/abs/2610.18206)
- [TurnCost (arXiv 2610.15093)](https://arxiv.org/abs/2610.15093)
- [ReTerm (arXiv 2610.18484)](https://arxiv.org/abs/2610.18484)
- [Self-Evolving Agent Harness (arXiv 2610.18813)](https://arxiv.org/abs/2610.18813)
- [TextualRL (arXiv 2610.18216)](https://arxiv.org/abs/2610.18216)
- [RBC — Reliable Memory from Unstable Growth (arXiv 2610.18427)](https://arxiv.org/abs/2610.18427)
- [ANIS — A Decade in a Day (arXiv 2610.15765)](https://arxiv.org/abs/2610.15765)
- [SPROUT (arXiv 2610.14405)](https://arxiv.org/abs/2610.14405)
- [Agentic Code Judging — ARBITER (arXiv 2610.18760)](https://arxiv.org/abs/2610.18760)
