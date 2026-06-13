# 面向岗位的项目介绍

## 项目名

MediaFlow Agent Go：AI 视频本地化与内容运营 Agent

## 一句话介绍

我用 Go 做了一个端到端 AI 产品原型，把“视频翻译、TTS 计划、语音克隆安全校验、内容生成、质量评分”串成可观测的 Agent 工作流，用来证明我能把模型能力变成真实可用的产品能力。

## 可以放简历的描述

- 基于 Go 标准库实现 AI 视频本地化 Agent 后端，支持 SSE 流式输出、工具注册、Agent Loop、OpenAI-compatible LLM 调用、Mock LLM 演示和文件持久化。
- 设计并实现 `analyze_brief / retrieve_knowledge / translate_script / build_tts_plan / generate_content_pack / score_quality` 工具链，将视频翻译、TTS、语音克隆审批和内容生成拆成可追踪步骤。
- 引入 Milvus 向量数据库、轻量 embedding、技能系统和 Guardrails，提升多场景适配能力，并对语音克隆、冒充、欺诈等风险请求进行拦截或标记。
- 为每次任务生成质量分、成功率代理、预计音频时长和成本估算，体现用产品指标持续优化模型效果的工程思路。

## 面试讲述版本

这个项目最初参考 baby-agent 的教学路径，但我没有照搬教程，而是把它改造成更贴近 AI 产品实习岗位的业务场景：视频本地化和内容生产。

用户输入一个视频脚本和目标渠道后，Agent 会先分析需求，再从 Milvus 向量库检索 TTS、短视频和产品指标经验，然后调用脚本本地化、TTS 计划、内容包生成和质量评分工具。整个过程通过 SSE 实时输出到浏览器，方便观察每个工具的输入、输出和耗时。

我特别关注“模型效果如何产品化”：不是只看生成文本好不好，而是把任务拆成质量分、预计成本、音频时长、成功率代理、人工复审要求等指标。这样后续可以比较不同 Prompt、模型或工具实现，持续优化体验、成功率和成本。

技术上我选择 Go，是因为它适合做高并发后端服务、部署简单、类型系统清晰。项目没有依赖重型 Web 框架，用标准库实现 HTTP、SSE、JSON、文件存储、Milvus REST 调用和并发编排；有 API Key 时可接真实 OpenAI-compatible 模型，没有 Key 时也能用 Mock 模型完整演示。

## 和岗位要求的对应关系

| 岗位要求 | 项目证明 |
| --- | --- |
| AI 产品研发与落地 | 完整 Web + Go 后端 + Agent 工具链 + Docker 部署 |
| TTS、视频翻译、语音克隆、内容生成 | 以视频本地化流程串联这些能力 |
| 模型效果量化到产品指标 | 质量分、成本、时长、成功率代理、复审要求 |
| 强编码能力 | 标准库实现 SSE、LLM Client、Milvus Client、工具注册和持久化 |
| AI Coding 效率 | 可说明从教程到业务项目的快速迁移过程 |
| 主动发现问题 | 主动加入 Guardrails、Mock Provider、可观测事件 |

## 后续可以扩展

- 接入真实 ASR、TTS 和视频渲染服务。
- 将 demo hash embedding 替换成真实 embedding 模型，并增加 Milvus filter / hybrid search。
- 增加评测集，对比不同 Prompt/模型的质量分和成本。
- 把 Job trace 接入 OpenTelemetry、Prometheus 和 Grafana。
