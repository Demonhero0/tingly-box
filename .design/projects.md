# Projects（协调者 + 线程）设计

> 调研：[`.research/claude-projects.md`](../.research/claude-projects.md)。本文是 tingly-project 对**新版 Claude Projects**（2026-09 重新设计）的复刻设计。
> 旧版"文件夹式 Projects"（知识库、项目对话）的实现被本设计取代并移除。

## 1. 产品定位

一个本地运行的"幕僚长"：用户把一串持续冒出来的工作丢进**一个对话**，协调者把它们拆成并行的**线程**（Claude Code 会话），
每个线程在自己的 git worktree 分支上干活，做完向协调者汇报。用户走开，回来只看 **Overview 里"等你处理"** 的部分，并在本地 review diff、合并。

和官方的差异定位（官方当前的空白，见调研 §7.8）：

| | 官方 | tingly-project |
|---|---|---|
| 线程跑在哪 | 云端（本地需 Remote Control） | **本机**，每个线程一个 git worktree |
| 模型 | 仅 Anthropic | 任意：通过 Tingly Box 网关路由，协调者和线程可分别选模型 |
| 代码托管 | 仅 github.com | 任意本地 git 仓库；有 GitHub remote 时可推送开 PR（P2） |
| 入口 | 网页、桌面、手机 | 本地网页（P2：IM） |

## 2. 体验要素（必须复刻）与 MVP 取舍

| 体验要素（调研 §4） | MVP | 说明 |
|---|---|---|
| 一个对话入口 + 协调者路由（就地回答 / 新线程 / 交给已有线程 / 一条多任务拆多线程） | ✅ | 协调者 = LLM + 工具 |
| 线程卡片出现在对话里（标题 + 状态，可点开） | ✅ | |
| Suggested threads（提议后由用户启动，可全部启动） | ✅ | `propose_threads` 工具 |
| 线程 = 完整 Claude Code 会话，独立分支 | ✅ | agentboot + git worktree |
| Overview：按状态分组，"Waiting on you" 为核心，按钮带小红点 | ✅ | |
| 打开线程：看过程、直接发消息、回答审批、停止 | ✅ | |
| 审批只在线程内回答；"仅此一次 / 本线程同类都允许" | ✅ | |
| 线程完成后向协调者汇报，协调者发更新 | ✅ | |
| 项目指令（≤16,000 字符，线程与协调者都收到） | ✅ | |
| 项目记忆 = 文件（`MEMORY.md` 索引 + 文件），Claude 自己写，设置里可看可改 | ✅ | 协调者 `remember/forget` 工具；线程直接写记忆目录 |
| 自然语言调节自主度，偏好写入记忆 | ✅ | 由协调者提示词 + 记忆实现，不做开关 |
| "缺东西就说缺什么然后停下"，线程需要输入时进入 Waiting on you | ✅ | 线程以 `NEEDS_INPUT:` 行结尾 |
| Goal（一句话目标） | ✅ | |
| 仓库（本地路径）、文件库 Library（上传文件 + 线程产出） | ✅ | |
| 协调者模型 / 线程模型分别设置 | ✅ | |
| Pause / Archive / Delete | ✅ | |
| 用量按线程 / 按模型 | ✅ 基础版 | 记录 token |
| Review：diff + 本地合并（"Merge it"） | ✅ | 本地版的 PR |
| 推送并开 GitHub PR、PR 自动修 CI / 回复 review | P2 | |
| Routines（定时） | P2 | |
| Setup recommendations / 首个项目自动探索线程 | P2 | |
| 从已有会话 "Continue as project" | P2 | |
| IM 通知与审批（imbot） | P2 | tingly-box 已有能力 |
| 共享 / 多用户 | ✗ | 官方也是单用户 |

## 3. 概念与命名（全局唯一，UX 原则 3）

- **Project**：一个长期目标及其工作流。
- **Conversation**（项目对话）：用户 ↔ 协调者的唯一对话。UI 文案：英文 "Conversation"，中文"项目对话"。
- **Coordinator**：项目对话里的 Claude。
- **Thread**：一个工作会话（Claude Code）。不叫 session/task/agent。
- **Overview**：线程状态面板。
- **Library**：上传文件 + 线程产出文件。
- **Memory**：记忆文件。**Instructions**：项目指令。**Goal**：一句话目标。

## 4. 架构

```
Browser ──REST──▶ HTTP modules (project / thread / settings)
   ▲                        │
   └──SSE /projects/:id/events ◀── Hub（项目内事件广播）
                            │
                 ┌──────────┴───────────┐
           Coordinator（每项目串行队列）   Thread Manager
           llm.Client + 工具循环          ThreadRunner（agentboot → claude CLI）
                 │                         git worktree / prompter / 汇报
                 └────── Store (SQLite) + 项目目录（memory/、library/、worktrees/）
```

### 4.1 项目目录（`$DATA/projects/<id>/`）
```
memory/MEMORY.md, memory/*.md   记忆（Claude 写，用户可改）
library/                        上传文件；线程产出写到 library/outputs/<thread-slug>/
worktrees/<thread-id>/          每个线程的 git worktree（无仓库时为空工作目录）
threads/<thread-id>.settings.json  线程的 Claude Code 设置（additionalDirectories 等）
```

### 4.2 数据模型
- `projects`: id, name, goal, instructions, repos(JSON `[{path, base_branch}]`), coordinator_model, thread_model, thread_permission_mode, paused, starred, archived_at, timestamps。
- `project_messages`: 项目对话。role(user|assistant|system), kind(text|thread_started|suggestions|thread_update|error), content, payload(JSON), created_at。
- `threads`: id, project_id, title, task, status, wait_reason, repo_path, base_branch, branch, worktree, session_id, model, last_report, commits_ahead, files_changed, input/output tokens, origin(coordinator|suggestion|user), timestamps, resolved_at。
- `thread_events`: 线程过程。kind(user|assistant|tool_use|tool_result|approval|ask|system|error|report), content, payload, created_at。

### 4.3 线程状态（与官方 Overview 分组一致）

| 状态 | 进入条件 | Overview 分组 |
|---|---|---|
| `working` | 一轮在跑 | Working |
| `waiting` | 有待回答的审批/提问；或上一轮以 `NEEDS_INPUT:` 结尾；或失败（`wait_reason` 说明） | **Waiting on you** |
| `ready` | 一轮结束，分支上有领先于基线的提交 | Ready for review |
| `idle` | 一轮结束，无待审内容 | Idle |
| `resolved` | 用户标记 / 合并后自动 / 一周无活动 | Resolved（可重开） |

（官方的 Landing 对应"PR 已批准排队合并"，本地合并是同步动作，MVP 不需要该状态。）

### 4.4 线程运行（ThreadRunner）
- 接口 `RunTurn(ctx, TurnSpec, prompter, sink) (TurnResult, error)`；生产实现用 `agentboot`（`claude` CLI，stream-json，`--resume` 续会话，`PermissionPromptTool: stdio`）；测试用 agentboot 的 fixture 工厂。
- **工作区**：项目有仓库 → `git worktree add -b tp/<slug>-<id6> <worktree> <base>`；没有仓库 → 空目录。
- **上下文注入**（`--append-system-prompt`）：项目名、Goal、Instructions、`MEMORY.md` 内容、记忆/文件库目录说明、线程规则：
  - 只做分配的这一件事；在当前分支上提交（commit），不 push、不改其他分支；
  - 需要的东西缺失时，说明缺什么并停下，不要 mock 或猜；
  - 需要用户决定才能继续时，最后一行写 `NEEDS_INPUT: <问题>`；
  - 结束时给出简短汇报：做了什么、怎么验证的、分支上有什么；
  - 记住项目级事实：在记忆目录写文件，并在 `MEMORY.md` 加一行索引。
- **目录权限**：生成 settings 文件，`permissions.additionalDirectories` = [library, memory, 其他仓库]，`--settings` 传入。
- **连接**：全局连接为 Anthropic 风格时，线程继承它（`ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN`），即默认走 Tingly Box；否则用本机 `claude` 自己的登录。
- **权限模式**：默认 `acceptEdits`（编辑自动通过，命令等需审批 → Waiting on you）；项目设置可改为 `auto` / `bypassPermissions`。
- **审批**：prompter 把请求记为线程事件、线程进入 `waiting`、阻塞等待用户在线程里回答（不超时，和官方一致）；"本线程同类都允许"按工具名记住。
- **消息排队**：线程忙时收到的消息（用户或协调者）排队，当前轮结束后依次执行。
- **一轮结束后**：读取 git 状态（领先提交数、改动文件数）→ 定状态 → 记一条 `report` 事件 → 通知协调者。

### 4.5 协调者
- 每个项目一个串行队列；触发源：用户消息、线程汇报（完成 / 需要输入 / 失败）。
- **按需构建上下文**（与官方一致：基于最近消息、线程表和记忆，而不是全量历史）：系统提示 = 角色 + Goal + Instructions + MEMORY.md + 仓库列表 + 线程表（id、标题、状态、最近汇报摘要）；消息 = 最近 N 条项目对话。
- **工具**：`start_thread`、`propose_threads`、`message_thread`、`get_thread`、`resolve_thread`、`remember`、`forget`、`update_instructions`。
- **路由规则**（提示词）：快速问题就地答；新工作开线程；同领域的后续交给已有线程并说明交给了谁；一条多任务拆多线程；用户表达的协调偏好（先提议、最多 N 个并行、更新简短……）用 `remember` 记下并遵守。
- 线程汇报触发的协调者回合：简短更新，只在完成、受阻或需要用户时发言。
- 并发上限：偏好由记忆表达；另有安全硬上限（默认同时 4 个运行中线程，超出排队）。

### 4.6 实时
`GET /api/v1/projects/:id/events`（SSE）：`message`（新增 / 更新，协调者流式文本）、`thread`（状态变化）、`thread_event`（线程过程新增）。前端进入项目页订阅，其余数据走 REST。

### 4.7 Review 与合并（本地版 PR）
- `GET /threads/:id/diff`：`git diff <base>...<branch>`（+ 未提交改动提示）。
- "Merge it"：仅当主仓库工作区干净且当前在 base 分支时执行 `git merge --no-ff <branch>`；否则给出原因和可复制的命令（UX 原则 11）。成功后线程 → `resolved`。
- 删除项目：移除 worktree，保留分支（与官方"不影响分支和 PR"一致）。

## 5. UI

- **项目页**：左侧项目对话（消息、线程卡片、提议卡片、输入框）；右侧 **Overview**（Threads 按状态分组 / Library / Memory 入口），点线程卡片或行 → 右侧切到线程详情（过程、审批、diff、操作、线程输入框）。头部：名称、收藏、Overview 开关（等你处理时有小红点）、设置。
- **项目设置**：General（名称、Goal、协调者模型、线程模型、权限模式、Pause / Archive / Delete）、Memory（Instructions + 记忆文件）、Environment（仓库）、Usage。
- **新建项目**：名称必填，Goal、仓库（本地路径，即时校验是否 git 仓库并显示当前分支）可选。
- 空项目首屏：教育内容内嵌（UX 原则 8）——说明"把任务丢进来、它会开线程、回来看等你处理"，并给 3 个可点的示例任务。

## 6. 与旧实现的关系
- 移除：知识库（`internal/knowledge`）、旧项目记忆（`internal/memory`）、项目内聊天。
- 保留：独立聊天（Chats）作为轻量功能，与项目解耦；设置页（模型连接）继续作为协调者的连接。

## 7. 验证
- 后端：fake LLM（脚本化工具调用）+ agentboot fixture（脚本化 Claude CLI 输出）+ 临时 git 仓库，覆盖：路由开线程、线程提交 → ready、审批 → waiting → 允许 → 继续、NEEDS_INPUT、汇报回到协调者、合并、暂停。
- 前端：typecheck / lint / vitest；无头浏览器走一遍主流程截图。
- 真机：本机 `claude` CLI 指向 fake Anthropic 服务跑通一个真实线程（P1 末）。

## 8. 状态（2026-10-11）

MVP 已实现（§2 中标 ✅ 的部分）。验证：
- 后端单测与 HTTP 端到端测试（含真实 Anthropic SDK 的 tool_use 流），`go test -race` 通过。
- **真机**：本机 `claude` CLI（2.1.296）作为线程，指向一个模拟 Anthropic 服务：协调者 `start_thread` → Claude Code 建 worktree 分支、请求 `Bash` → 线程进入 Waiting on you → 在 UI 允许 → 提交 → Ready for review → diff → Merge 落到真实仓库。

已知问题：
- **agentboot 进程句柄的竞态**（上游 tingly-box）：`process/osexec.go` 启动后立即在 goroutine 里 `cmd.Wait()`，而 `Wait` 会关闭 stdout 管道；CLI 在写完 result 后立即退出时，解码器最后一次读会报 `read |0: file already closed`。`os/exec` 文档明确要求读完管道再调用 `Wait`。tingly-project 在 `internal/project/runner.go` 中只对"已收到成功 result + 该错误"这一种情况容错；根治应在 agentboot 修复。
