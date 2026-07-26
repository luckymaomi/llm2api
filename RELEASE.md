# LLM2API 0.1.0 发布候选说明

`RELEASE.md` 随发布包交付，只回答“这个版本包含什么、验证到哪里、还缺什么”。产品与系统事实以 [spec.md](spec.md) 为准，开发验收边界以 [dev.md](dev.md) 为准，正式操作以 [deploy/README.md](deploy/README.md) 为准。

## 本次实现中的新增事实

- 候选上游 API Key 已持久化管理员 `priority` 与同级 `weight`：调度器先选最小优先级，再在同级按权重分配；冷却、不可用和容量拒绝仍按原有恢复边界处理。
- 同一 Provider 的官方账号/项目可由一个共享上游范围统一持有 RPM、TPM、并发与可选 TPD/UTC 重置配置。上游 API Key 仅引用该范围，不复制额度事实；固定日额度和分钟桶在一次 Valkey 脚本中原子扣减，权威 usage 差额幂等回补，控制台同时投影单 Key 网关余量、共享范围网关余量和官方余额/未知状态。
- 能力目录仍以 `/v1/models` 为准：网关只声明当前公共协议可以无损表达的能力。无法表达的厂商接口形态不进入声明，或在网络发送前以稳定 4xx 拒绝。
- 外部 Provider 身份固定为 `llm2api`：模型 `owned_by`、公共错误、成功响应 ID/模型名、`llms.txt` 与 OpenAPI 不再暴露真实 Provider、资源池或上游凭据；逐模型 adapter、套餐资格和同池调度仍在网关内部完整执行。
- 模型能力持久化和公共目录已补齐 reasoning 配置/内容、消息 `name`、非流/流式 usage、推理档位、工具选择条件与逐参数约束；Chat 流式 usage 通过标准 `stream_options.include_usage` 申请。

## 状态

`0.1.0` 当前是发布候选，不等于已经在正式生产环境投产。构建成功、隔离验收或本机容量通过都不能代替正式域名、主机、真实外部监控、异地备份和目标环境恢复证据。

## 本版本包含

- OpenAI-compatible Models、Chat Completions 与 Responses，覆盖非流、SSE、工具、reasoning、usage、取消和稳定错误。
- Agnes、智谱 GLM、硅基流动与 Kimi 专用 adapter；模型能力以公开 capability matrix 为准并保真转发。
- 首位管理员一次性初始凭据、自助换密、管理员直接创建成员、成员权限、API 密钥、重叠更换和账号恢复。
- 代码内置 Provider catalog、逐模型 capability matrix、资源池与上游 API Key 管理、不可变路由套餐、有效期订阅、同池多 Key 接管、跨池隔离和有界恢复；上游 API Key 使用一个逐行粘贴入口同时添加一条或多条，并可手动获取官方上游状态。
- PostgreSQL 持久请求与 usage 事实、Valkey 短期容量协调、Prometheus/Grafana 观测和脱敏审计；不包含价格、币种、成本换算或成本汇总域。
- Linux 双 Gateway/Caddy TLS Compose、Windows SCM、独立 migration、滚动升级、加密备份和空环境灾备工具。
- Windows/Linux amd64 压缩包、Linux OCI tar、许可证清单、SPDX/CycloneDX SBOM、checksum、manifest、provenance 与签名 bundle。

## 已验证边界

| 目标 | 已验证事实 | 仍需目标环境证明 |
| --- | --- | --- |
| Windows amd64 | Go 1.26.5、原生 SCM、Event Log、停止/重启与有界失败恢复 | 正式 TLS 前置代理 |
| Linux amd64 容器 | scratch Gateway、PostgreSQL 18.4、Valkey 9.1.0、Caddy 2.10.2 双实例隔离拓扑 | 正式主机、DNS、证书与镜像仓库 |
| 管理端 | 生产前端构建与真实 HTTP 合同连接 Go/PostgreSQL/Valkey；桌面 UI 由 owner 人工验收 | owner 的视觉验收与正式站点 |
| 上游探测 | 本机控制台通过正式解密、SSRF-safe transport 与 Agnes adapter 收到一次真实成功响应；隔离旅程持续验证稳定结果和脱敏 | 目标 Provider 当日网络、合同、额度与全套标准 SDK |
| 标准客户端 | OpenAI Go v3.44.0、Python openai 2.46.0 | 目标 Provider 当日额度和网络 |
| 容量 | 300 个受控用户 profile、900 秒稳态、突发、长流与强杀恢复 | 目标硬件的重新测量与长时 soak |
| 灾备 | 隔离的加密快照、空环境新库恢复与主旅程复验 | 异地仓库、真实告警与现场 RPO/RTO |

未列入此版本目标：Linux arm64、macOS 服务、Kubernetes、多地域高可用，以及图像、视频、语音、Embedding 和 Rerank 协议。

## 使用与部署

- 从源码在 Windows 首次体验：按 [README.md](README.md) 运行 `python .\start_dev.py`。
- 正式 Linux/Windows 安装、升级、回滚、备份和恢复：只按 [deploy/README.md](deploy/README.md) 操作。
- 发布包必须先验证 checksum、provenance subject、SBOM 和签名身份；任一失败都拒绝部署。
- production 镜像必须使用已审核的 `@sha256`，不得改用浮动标签。

## 正式发布前仍需完成

- 公开 GitHub Actions 与正式 tag 的 OIDC keyless 发布身份现场通过。
- 在指定生产主机连接正式域名、DNS、TLS、真实 Provider、PostgreSQL、Valkey 和生产前端。
- 接入外部 Prometheus/Grafana、通知渠道和证书/备份新鲜度告警。
- 使用异地加密仓库完成备份、完整性检查和新主机恢复演练。
- 用管理员、成员、标准 SDK 覆盖登录、刷新、取消、强杀、升级、回滚和灾备切流，并记录网络、日志、指标、持久状态与制品证据。
