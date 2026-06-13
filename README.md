# MediaFlow Agent Go

MediaFlow Agent Go 是一个用 Go 实现的 AI 视频本地化与内容运营 Agent。它把视频脚本本地化、知识检索、TTS 执行计划、内容包生成、安全校验和质量评分串成一条可观测的工作流，并通过浏览器界面实时展示执行过程。

## 项目特性

- **端到端 Agent 工作流**：输入一段视频/脚本需求，Agent 自动完成需求分析、知识检索、脚本本地化、TTS/语音克隆执行计划、内容包生成和质量评分。
- **端到端全栈**：Go 后端提供 Agent 服务、SSE 流式事件、文件持久化；`web/` 提供可交互的浏览器界面。
- **可观测工具链**：把翻译、TTS、语音克隆、内容生成拆成独立工具，每个步骤都有耗时、结果和质量指标。
- **工程化组件**：内置 Guardrails、技能系统、轻量 RAG、Mock LLM、OpenAI-compatible LLM 客户端、Job 存储，方便从 demo 迭代到生产。
- **向量数据库**：支持 Milvus standalone，启动时把 `knowledge/` 导入向量库，Agent 检索时优先走 Milvus 召回。

## 核心模块

| 模块 | 实现 |
| --- | --- |
| LLM Client | `internal/llm/openai.go` 实现 OpenAI-compatible 调用 |
| Agent Loop | `internal/agent` 调度 `internal/tools` 工具链 |
| SSE Streaming | `/api/agent/run` 以 SSE 输出状态、工具调用和结果事件 |
| Persistence | Job 文件持久化 + 每次执行 trace 化 |
| RAG | `retrieve_knowledge` 优先从 Milvus 检索，失败时回退 `knowledge/` |
| Skills | `skills/*.md` 加载场景技能 |
| Guardrails | `internal/guardrails` 检查语音克隆、冒充、欺诈等风险 |
| Metrics | `score_quality` 输出质量分、成功率代理、成本和时长风险 |

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

不配置 `LLM_API_KEY` 时，项目使用 deterministic demo provider，方便在无外部模型服务的情况下完整跑通链路。配置 OpenAI-compatible 服务后，脚本本地化和内容包生成会优先调用真实模型。

### Linux + Docker 运行

Linux 机器只需要安装 Docker 和 Docker Compose v2：

```bash
git clone https://github.com/roookiiee/mediaflow-agent-go.git
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

## 设计说明

MediaFlow Agent Go 不依赖重型 Web 框架，后端主要使用 Go 标准库实现 HTTP 服务、SSE 流式传输、JSON 编解码、文件持久化和 Milvus REST 调用。项目默认提供 Mock LLM 和本地 hash embedding，因此没有 API Key 或外部 embedding 服务时也能运行；接入真实模型后，可以把同一套工具链用于更高质量的脚本本地化和内容生成。

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
docs/                      项目说明与扩展材料
docker-compose.yml         Linux 一键启动 Go 服务 + Milvus
```
