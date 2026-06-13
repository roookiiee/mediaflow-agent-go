# MediaFlow Agent Go

面向 AI 产品实习岗位的 Go 项目：一个「视频本地化与内容运营 Agent」原型。它把 `baby-agent` 教程里的 Agent Loop、工具调用、技能、轻量 RAG、Guardrails、SSE Web 服务和可量化指标，改造成一个能演示真实产品闭环的作品集项目。

## 为什么适合这个岗位

- **AI 产品落地**：输入一段视频/脚本需求，Agent 自动完成定位分析、知识检索、脚本本地化、TTS/语音克隆执行计划、内容包生成和质量评分。
- **端到端全栈**：Go 后端提供 Agent 服务、SSE 流式事件、文件持久化；`web/` 提供可交互的浏览器界面。
- **模型能力产品化**：把翻译、TTS、语音克隆、内容生成拆成可观测的工具链，每个步骤都有耗时、结果和质量指标。
- **工程化意识**：内置 Guardrails、技能系统、轻量 RAG、Mock LLM、OpenAI-compatible LLM 客户端、Job 存储，方便从 demo 迭代到生产。
- **向量数据库**：支持 Milvus standalone，启动时把 `knowledge/` 导入向量库，Agent 检索时优先走 Milvus 召回。
- **AI Coding 叙事**：项目结构清晰，适合在简历/面试里说明如何用 Codex/Cursor/Claude Code 快速从教程迁移成业务项目。

## 功能对应 baby-agent

| baby-agent 能力 | 本项目落点 |
| --- | --- |
| Chat Completions / Raw HTTP | `internal/llm/openai.go` 实现 OpenAI-compatible 调用 |
| Tool Calling / Agent Loop | `internal/agent` 调度 `internal/tools` 工具链 |
| SSE 流式输出 | `/api/agent/run` 以 SSE 输出思考、工具、结果事件 |
| Context / Memory | Job 文件持久化 + 每次执行 trace 化 |
| RAG | `retrieve_knowledge` 优先从 Milvus 检索，失败时回退 `knowledge/` |
| Skills | `skills/*.md` 渐进式加载场景技能 |
| Guardrails | `internal/guardrails` 检查语音克隆/冒充等风险 |
| Eval / Metrics | `score_quality` 输出成功率代理、成本和时长风险 |

## 快速开始

### 本地 Go 运行

```bash
cd mediaflow-agent-go
cp .env.example .env
go run ./cmd/mediaflow-agent
```

打开浏览器访问：

```text
http://localhost:8080
```

不配置 `LLM_API_KEY` 时，项目使用 deterministic demo provider，适合演示项目链路。配置 OpenAI-compatible 服务后，脚本本地化和内容包生成会优先调用真实模型。

### Linux + Docker 运行

另一台 Linux 机器只需要安装 Docker 和 Docker Compose v2：

```bash
git clone <your-repo-url>
cd mediaflow-agent-go
docker compose up --build
```

启动后访问：

```text
http://localhost:8080
```

Compose 会同时启动：

- `mediaflow-agent`：Go Web + Agent 服务
- `milvus-standalone`：Milvus 向量数据库，端口 `19530`
- `milvus-etcd`：Milvus 元数据
- `milvus-minio`：Milvus 对象存储，控制台端口 `9001`

Docker 模式默认设置：

```text
MILVUS_ENABLED=true
MILVUS_ADDRESS=http://standalone:19530
MILVUS_COLLECTION=mediaflow_knowledge
```

服务启动时会自动创建 Milvus collection，并把 `knowledge/*.md` 写入向量库。为了保证没有外部 embedding 服务也能直接跑通，项目内置了一个确定性 hash embedding；真实上线时可以把 `internal/vector/embedding.go` 替换成 OpenAI、BGE、Jina 或自部署 embedding 模型。

## API

```bash
curl -N -X POST http://localhost:8080/api/agent/run \
  -H "Content-Type: application/json" \
  -d '{
    "brief": "把一条 45 秒中文产品介绍视频本地化成英文 TikTok 版本，需要 TTS 和标题文案",
    "source_script": "大家好，今天介绍一款可以自动整理会议纪要的 AI 工具...",
    "target_locale": "English",
    "channel": "TikTok"
  }'
```

## 面试讲法

> 我基于 baby-agent 的教学思路，用 Go 做了一个 AI 视频本地化 Agent。它不是简单聊天机器人，而是把一个真实内容生产流程拆成多个可观测工具：需求分析、知识检索、脚本本地化、TTS 计划、内容包生成和质量评分。后端用 Go 标准库实现 SSE 流式服务、工具注册、Guardrails 和文件持久化；没有 API Key 也能用 Mock LLM 演示，有 Key 时可以接 OpenAI-compatible 模型。这个项目主要想证明我能把模型能力拆成稳定产品链路，并且能用指标持续优化体验、成功率和成本。

## 目录

```text
cmd/mediaflow-agent/       HTTP 服务入口
internal/agent/            Agent 编排与 trace
internal/tools/            产品工具链
internal/llm/              Mock + OpenAI-compatible LLM 客户端
internal/skills/           技能发现与加载
internal/guardrails/       安全边界
internal/store/            Job 文件持久化
knowledge/                 RAG 知识库
skills/                    场景技能
web/                       浏览器交互界面
docs/                      简历与岗位介绍材料
docker-compose.yml         Linux 一键启动 Go 服务 + Milvus
```
