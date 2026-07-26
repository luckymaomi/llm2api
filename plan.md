# LLM2API 单一外部 Provider 与 Kitty 端到端接入

## 需求文档

- 用户与场景：受控成员和任意外部 Agent 只把 LLM2API 视为唯一 Provider，使用一个 Base URL、一把下游 API 密钥和套餐授权的模型；管理员在网关内部维护真实 Provider、资源池与上游 API Key。
- 要解决的问题：网关虽已具备 canonical model、Provider adapter、资源池调度和套餐资格链，但公共 `/v1/models` 与错误曾暴露真实 Provider，能力目录也未投影全部已持久化能力；Kitty 此前没有只依赖 LLM2API 公共合同的 Provider profile。
- 当前阶段范围：收紧 LLM2API 公共身份边界，补全机器可读能力合同及其请求/响应闭环，证明套餐驱动的内部路由与 Provider 转换；随后在 Kitty 保留原生 Agnes、智谱等独立 Provider 的同时，新增唯一通用 `llm2api` Provider adapter，并以临时下游 API 密钥真实调用 Agnes。
- 可验收完成标准：
  - 公共 API、公共错误、SDK 可见响应和 API 文档只声明 `llm2api`，不泄露真实 Provider、资源池、上游 API Key 或内部候选。
  - `GET /v1/models` 只返回当前下游 API 密钥经活动成员、活动订阅和不可变套餐版本授权的模型，并完整描述客户端需要的模型能力与参数限制。
  - `POST /v1/chat/completions` 对每项公开且已声明支持的能力无损进入 canonical model，再由实际 Provider adapter 转成上游 wire；不支持或无法无损表达的字段在上游发送前结构化拒绝，不能静默丢弃。
  - 内部仍按套餐授权资源池、priority、weight、局部与共享容量、冷却和熔断选择合法上游 API Key；外部 Agent 不参与也不知道该选择。
  - Kitty 只配置 `KITTY_PROVIDER=llm2api`、网关 Base URL、下游 API 密钥和模型；LLM2API 模式下以 `/v1/models` 为模型发现和能力 owner，不在 Kitty 内为中转后的 Agnes、智谱或其他上游模型增加静态特判。
  - LLM2API 确定性验证、统一核心旅程、真实 Provider 验收、Kitty 完整验证以及 Kitty -> LLM2API -> Agnes 真实主旅程均通过。

## 当前事实

- 已确认的源码、数据、配置、测试和运行事实：
  - 公共入口已有 `GET /v1/models`、`POST /v1/chat/completions` 和可映射的 `POST /v1/responses`；HTTP 请求先解析到 `internal/canonical`，再进入鉴权、套餐资格、admission、路由和 Provider adapter。
  - Provider catalog 当前拥有 Agnes、Kimi、硅基流动和智谱的精确模型能力；adapter 已分别处理 thinking/reasoning、工具、流式 usage、图片/视频、结构化输出及模型特有参数。
  - 调度已具备资源池硬隔离、管理员 priority、同级 weight、Key 局部容量、账户/项目共享 RPM/TPM/并发/TPD、冷却、熔断、有界重试和未知副作用保护。
  - `/v1/models` 当前把真实 `ProviderSlug` 写入 `owned_by`；公共结构化错误拥有 `provider` 字段，部分路径会写入真实 Provider。
  - 持久模型能力已有 `reasoning_config`、`reasoning_content`、`message_name` 和 `stream_usage`，但当前公共模型能力投影没有完整暴露这些字段。
  - Kitty 原先只认识 Agnes、Google、DeepSeek、智谱和通用 OpenAI-compatible profile；通用 profile 无法证明 LLM2API 动态授权模型，因此本次保留 Kitty 原生 Agnes、智谱等独立 Provider，并新增通用 `llm2api` profile。LLM2API 模式下模型存在性和能力来自 LLM2API `/v1/models`，Kitty 不复制上游模型特判。
- 已核验的外部合同：
  - Portkey Gateway `main`（2026-07-26 调查，MIT）证明一个网关入口、虚拟凭据和内部多 Provider 路由可作为外部 Agent 的单一服务面；只研究机制，不复制源码。
  - LiteLLM 当前公开仓库（2026-07-26 调查）展示统一 OpenAI 入口和内部 deployment 路由；其自定义 Provider 发现仍可能要求客户端提供 Provider 前缀，这种内部身份泄漏明确不采用。
- 未知项及其验证方式：公共错误、响应 header、Responses 续接和 OpenAPI 是否还有 Provider 泄漏，使用结构化源码调查与真实 HTTP 合同验证；所有模型能力是否逐项贯通，使用 catalog 驱动的合同矩阵和真实 Provider canary 验证。
- 工作区已有改动与保护边界：当前 LLM2API 工作区已有本任务前序的能力持久化、priority/weight、共享容量、TPD、前端和文档改动，全部保留并在同一终局下验收；不提交、不推送、不部署。Kitty 初始工作区为 clean，修改只在 LLM2API 公共合同稳定后开始。

## 失败证据

- 失败仍存在时可观察的产品结果：
  - 带有效下游 API 密钥调用 `/v1/models` 会得到 `owned_by=<真实 provider_slug>`，外部 Agent 能识别内部 Provider。
  - 某些公共 4xx/5xx 响应中的 `error.provider` 会返回真实 Provider。
  - 外部 Agent 无法从 `/v1/models` 判断 reasoning 配置/内容回放、消息名称和流式 usage 的完整支持状态。
  - Kitty 配置 `KITTY_PROVIDER=llm2api` 会因未知 Provider 失败；改用 `openai-compatible` 又会降级模型能力。
- 可重复的测试、命令或日志证据：`internal/publicapi/api_test.go` 当前明确期待 `"owned_by":"zhipu"`；`internal/publicapi/models.go` 使用 `model.ProviderSlug`；Kitty `src/provider/catalog.ts` 不含 `llm2api` profile。

## 最终目标

- 当前阶段完成后的生产级终局：
  - 外部数据流固定为 `Agent -> LLM2API 公共协议 -> 下游 API 密钥 -> 活动成员/订阅/套餐版本 -> 授权模型/资源池 -> 合格上游 API Key -> Provider adapter -> canonical 响应 -> Agent`。
  - 管理面和内部观测保留真实 Provider 与资源事实；公共面只暴露 LLM2API 身份、授权模型、能力、用量、稳定错误与请求 ID。
  - Kitty 的 LLM2API profile 只发送 LLM2API canonical 公共字段；`/v1/models` 负责授权、可用性探测和动态模型能力，Kitty catalog 只保存 LLM2API Provider 的公共接线事实，不保存 Agnes、智谱等中转模型的静态能力。
- 必须保持的不变量：每项事实只有一个 owner；Provider 差异不进入公共 handler、Kitty 或调度；套餐未授权资源池绝不候选；已提交或部分流不盲目重放；secret 不回显、不记录、不进入审计或测试日志。

## 不做范围

- 不把真实 Provider 名称、Provider 前缀、资源池 ID、上游 API Key 或候选列表作为公共路由参数。
- 不增加 Provider 原始请求 passthrough、任意扩展 JSON 或未知字段透传；只有 canonical model 明确拥有且能力目录声明的字段可以发送。
- 不把当前 Provider 未验证的音频、Embedding、Rerank 等能力伪装为支持；能力扩展必须先进入 catalog、canonical、adapter、测试和文档。
- 不建立 LLM2API 与旧 Kitty Provider 配置的兼容别名、双 adapter 或模型名推断。
- 不在本任务执行 commit、push、部署或生产数据变更。

## 设计

### 事实 Owner

- 公共协议：`internal/protocol` 拥有唯一 LLM2API OpenAI-compatible wire；`internal/publicapi` 只投影 `llm2api` 服务身份。
- 状态与持久化：Provider catalog 拥有验证能力，registry 持久化其完整投影，套餐版本与订阅拥有模型/资源池资格，账本拥有额度和 usage。
- 外部接线：Kitty `src/provider` 中唯一 `llm2api` profile 拥有网关 wire；`/v1/models` 证明授权与可用性，Kitty catalog 只保存客户端发送所需的公共模型合同，不复制资源池、上游 Key 或内部 Provider 路由。
- 错误与恢复：canonical error kind 决定公共错误和安全重试；公共 presenter 清除内部 Provider 身份；上游失败、冷却、熔断和候选切换仍由 LLM2API requestflow 拥有。
- 展示与可观测性：外部只看 LLM2API、模型、能力、请求和 usage；管理员控制台、内部指标和审计继续看真实 Provider/资源池/Key 脱敏事实。

### 数据与控制流

- 请求进入、状态提交、外部副作用、结果结算和用户输出：公共 wire 严格解析到 canonical；资格与 admission 成功后持久接受；路由从套餐授权资源池内选 Key；adapter 生成实际上游 wire；响应规范化、usage 结算后只用公共 LLM2API 合同返回。
- 事务、并发、幂等、重试和 uncertain 边界：继续复用当前请求接受、Valkey 原子容量、执行 claim、attempt 和 usage owner；未发送可安全换 Key，已提交/部分流/未知副作用不自动重放；重复请求依赖幂等键返回同一事实或稳定冲突。

### 安全与数据

- 下游 API 密钥只鉴权 LLM2API；上游 API Key 只在 adapter 发送边界短暂解密使用。公共 body、header、错误、模型目录、日志和 Kitty 配置不包含真实 Provider 凭据或内部路由身份。
- `key.txt` 只由隔离真实 Provider 验收在进程内读取，不输出、不提交、不复制到 Kitty；Kitty 使用临时生成的下游 API 密钥，验收结束后撤销或删除隔离状态。

### 关键取舍

- 采用方案及证据：稳定 LLM2API 服务身份 + 每模型动态能力目录 + canonical 严格协议 + 内部 adapter/资源池路由，能同时满足能力完整性和内部身份隔离。
- 被拒绝方案及原因：Kitty 使用 Agnes adapter 但把 URL 指向网关会泄漏内部 Provider 语义；通用 OpenAI-compatible profile 会降级能力；Provider 原始字段透传会破坏唯一公共合同和发送前校验。

## 生产级切片

- [x] 切片 1：公共 LLM2API 身份、完整模型能力目录、公共错误脱敏与 OpenAPI/文档形成一致合同。
- [x] 切片 2：catalog 驱动地证明每个已声明 Chat 能力从公共 wire 到 canonical、Provider wire、canonical 响应和 usage 均无静默丢失，恶劣路径保持原有恢复边界。
- [x] 切片 3：Kitty 保留原生 Agnes、智谱等独立 Provider，新增唯一通用 `llm2api` adapter、动态模型能力发现、配置/UI/文档和安全重试闭环；LLM2API 模式不保存内部 Provider 或上游模型特判。
- [x] 切片 4：真实 PostgreSQL/Valkey/Gateway/生产前端核心旅程以及 Kitty -> LLM2API -> 内部实际 Provider 真实验收闭环；验收断言只使用 LLM2API 公共事实。

## 实施任务

- [x] 完成当前公共协议、canonical、Provider adapter、资源池调度和 Kitty Provider owner 的全局核心语义调查
- [x] 固化真实 Provider 身份泄漏、能力目录缺项和 Kitty 未接入的失败证据
- [x] 收紧所有公共模型、错误、响应、header、Responses 和 OpenAPI 的 LLM2API 单一身份合同
- [x] 补全 `/v1/models` 能力投影并建立 catalog 驱动的持久能力重建合同矩阵
- [x] 建立 catalog 驱动的公共协议到 Provider wire、响应与 usage 合同矩阵
- [x] 复核并修复所有已声明能力的请求接受、发送、响应、流式、usage 与拒绝路径
- [x] 同步 LLM2API 管理端、README、spec、RELEASE 和 API 文档
- [x] 保留 Kitty 原生 Agnes、智谱等独立 Provider；新增唯一通用 LLM2API adapter。LLM2API 模式下以 `/v1/models` 为模型能力 owner，不在 Kitty 内为中转后的 Agnes 等模型增加静态特判。
- [x] 让 Kitty 配置、请求、重试、工具流选择和运行时模型限制只消费 LLM2API 公共模型合同
- [x] 运行 LLM2API 与 Kitty 定向测试、完整验证、统一核心旅程和真实端到端验收
- [x] 检查两仓库差异、公共响应与 Kitty 输出的内部 Provider 泄漏、敏感信息和临时密钥清理

## 恶劣路径矩阵

| 边界 | 接受/提交事实 | 失败状态 | 恢复 owner | 重放与幂等 | 验证证据 |
| --- | --- | --- | --- | --- | --- |
| 重复请求/重复操作 | 请求接受与幂等指纹持久化 | 同键异体稳定冲突 | execution/request owner | 同键同体返回既有事实，不重复扣费 | 统一核心 HTTP 旅程 |
| 客户端断连/取消 | admission 或 attempt 状态已知 | canceled、uncertain 或 stream_interrupted | requestflow/execution | 未发送可释放；已提交不盲重放 | 流式断连与取消测试 |
| 上游超时、429、5xx | attempt 与容量租约已记录 | cooldown、rate_limit、provider_temporary | health/routing/requestflow | 仅安全边界内同池接管 | Provider 与故障测试 |
| 部分流/未知副作用 | 首个已提交事件标记边界 | stream_interrupted/uncertain | requestflow + usage | 禁止换 Key 重放，usage 按可观察事实结算 | 畸形流、部分流测试 |
| 并发额度竞争 | Key 局部与共享容量原子获取 | admission_capacity_exhausted | coordination | 原子退款/结算且幂等 | PostgreSQL/Valkey 并发测试 |
| 进程强杀/主机重启 | request/attempt/claim 持久化 | stale running/uncertain | recovery worker | claim fencing，重复恢复幂等 | core restart recovery |
| 存储或协调设施故障 | 无法证明接受时失败关闭 | storage/admission unavailable | store/coordination | 不绕过额度或资格 | core fail-closed |
| 资源写入或套餐发布竞态 | 事务提交后唯一事实生效 | optimistic conflict | registry/subscription owner | 幂等写入，不双轨读取 | 控制面 HTTP 合同 |
| 公共身份泄漏 | 仅提交 `llm2api` 身份 | 公共响应不得出现内部 Provider | public presenter | N/A | models/errors/OpenAPI 真实 HTTP 合同 |

## 验证计划

- 定向检查：`go test` 覆盖 protocol、publicapi、providers、requestflow、registry、routing、coordination；Kitty 覆盖 provider catalog/adapter/config 与真实 host turn。
- 完整验证：LLM2API `go test ./...`、`go build ./...`、`web pnpm.cmd run verify`、`git diff --check`；Kitty `npm.cmd run verify`。
- 竞态/并发验证：LLM2API coordination、routing、usage 和执行恢复的现有并发测试；必要时运行 `go test -race` 的定向包。
- 目标平台构建：Windows PowerShell、Go/sqlc、Node/pnpm/npm；统一入口 `python .\start_test.py daily` 与 `core`。
- 隔离的真实 Provider 验收：`powershell -ExecutionPolicy Bypass -File .\scripts\test-provider-real.ps1 -KittyRepository ..\kitty -AgnesPoolOnly` 从 `key.txt` 结构化提取候选并用 Agnes 官方 `/models` 逐把确认，创建临时下游 API 密钥，再从 Kitty 走 LLM2API 调用 `agnes-2.0-flash`，验证 `/models`、thinking、流式 usage、多轮对话、非流式工具调用和完整 Agent turn。
- 安全与敏感信息检查：检查 git diff、公共响应和日志不含 key、真实 Provider/internal route 泄漏；验收后清理临时下游密钥和隔离状态。

## 收口

- 完成事实：LLM2API 公共身份、能力目录、逐模型内部 adapter、池内调度、确定性验证、Kitty 通用 `llm2api` 接入和真实 Kitty -> LLM2API -> Agnes 主旅程均已闭环。Kitty 保留原生 Agnes、智谱等独立 Provider；LLM2API 模式只消费 `/v1/models` 动态模型与能力，不保存中转上游模型静态特判。
- 实际命令与结果：LLM2API `python .\start_test.py daily` 通过，日志为 `.build\test-logs\20260726T110545706824Z-daily.log`；Kitty `npm.cmd run verify` 在 `4.0.13` 通过；`powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-provider-real.ps1 -KittyRepository ..\kitty -AgnesPoolOnly` 通过，真实请求经临时下游 API 密钥完成 `/models`、动态能力、stream smoke、多轮、上下文压力、后台、Playwright 浏览器、修复任务与 LLM2API 账本核对，最终记录 `requests=50`、`distinctUpstreamKeys=3`、`authoritativeTokens=276649`。两仓库 `git diff --check` 通过。
- 未验证项：控制台桌面视觉仍需 owner 人工验收；未运行 LLM2API `full/capacity/release/everything` 长档。
- 剩余风险：Provider 外部合同持续变化；只能对 catalog 已验证并由 `/v1/models` 声明的能力承诺无损转发。真实密钥已用于本机隔离验收但曾在对话中暴露，建议 owner 另行轮换。
- commit/push/部署状态：owner 已授权本次 Kitty 升级 npm 版本、推送并发布；LLM2API 推送但不发布 npm。
