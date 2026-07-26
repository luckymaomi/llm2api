# 真实网关地址与额度可观测性调研

## 需求文档

- 用户与场景：管理员从控制台复制 OpenAI-compatible 配置给 Kitty 等客户端；同时需要判断每条合法上游 API Key 是否已经用尽可用额度。
- 要解决的问题：开发控制台运行在 5173，而 Gateway 运行在 8080；当前控制台以浏览器地址生成 Base URL，会把 5173 错写为 Gateway 地址。
- 当前阶段范围：交付真实公共 Gateway 地址；同时交付已配置 RPM/TPM/并发的实时容量投影、Provider usage 的安全回补，以及 Key 级别的上游额度证据展示。不接入未被官方文档授权的余额/订阅查询，不读取 secret，也不调用真实 Provider。
- 可验收完成标准：本地 `start_dev.py` 默认启动后，控制台、替换 API 密钥配置、`/llms.txt` 和 `/openapi.json` 一致给出 `http://127.0.0.1:8080/v1`；变更 Gateway 端口或配置的公共 Origin 后，四处随之更新；每条上游 API Key 可看到网关实际追踪的 RPM/TPM/并发余量、上游健康/冷却/额度拒绝证据及其含义；Provider 已报告的实际 Token 小于预留值时，余量被幂等回补。

## 当前事实

- 已确认 `start_dev.py` 默认 Gateway 端口为 8080、控制台端口为 5173；当前机器两端口均在监听。
- `web/src/features/docs/api-docs-page.tsx` 与 `web/src/features/access/key-replacement-dialog.tsx` 用 `window.location.origin` 生成 API Base URL，开发模式错误地得到 5173。
- Vite 只将 `/api` 与 `/v1` 代理到 Gateway；5173 是开发控制台，不是客户端应依赖的 Gateway 入口。
- `internal/httpserver/documentation.go` 从请求 Host 构造 `/llms.txt` 和 OpenAPI 的 server URL，尚未使用显式公共地址事实。
- 当前配置只有监听地址 `LLM2API_HTTP_ADDRESS`，没有可对外调用的公共 Origin；当前工作区无未提交改动、没有现有 `plan.md`。
- 上游额度通常按账户、项目或权益结算而非单条 API Key；当前仓库只实现 RPM/TPM/并发本地预留、凭据冷却和错误分类，未持久化上游周期额度、余额或重置事实。
- 已确认 Agnes 同类型 Key 共享额度池；SiliconFlow 按账户和模型结算；智谱按用户权益及模型结算。三个事实都否定了将多个 Key 的额度简单相加。
- 已确认当前 Rate bucket 在 admission 时按 `EstimatedTokens` 扣减，但成功响应得到 Provider usage 后没有回补；路由仍为随机等价轮转，控制台不展示 credential 的 health status。

## 失败证据

- 控制台“接口文档”显示 `http://127.0.0.1:5173/v1`，其 cURL、Agent 配置与模型列表示例因而将直连客户端指向错误的入口。

## 最终目标

- `config.HTTP.PublicOrigin` 是 Gateway 公共 Origin 的唯一 owner；所有对外 API 地址由它派生。
- 开发环境默认从确定的 loopback 监听地址得到公共 Origin；生产环境要求显式、合法的公共 Origin，不能将 bind address、浏览器 Origin 或请求 Host 猜作公开地址。
- 控制台通过只读控制 API 获得公开 API Base URL、Agent 索引和 OpenAPI URL；无法读取时给出明确失败状态，不产生假地址。
- 机器可读文档以同一配置生成 URL。下游 API Key、上游 secret、请求正文和真实 Provider 调用均不进入此切片。
- 协调层拥有网关已配置容量的实时余量；registry 拥有上游凭据健康、冷却、最后成功和错误种类。控制 API 仅组合二者为只读投影，不能把它持久化为第二套额度事实。
- 仅当 Provider 已返回实际 usage，且实际 token 小于先前预留时，协调层按 execution ID 幂等回补差额；未知、估算、流中断和上游结果未知不回补。

## 不做范围

- 不在本切片接入上游余额、订阅配额或任何未经官方 API 授权的查询方式。
- 不为了保持 5173 可调用而保留错误的客户端文档或代理依赖。
- 不修改资源池隔离、路由/调度策略、账本或成员 API Key 合同。
- 不把 Key 数量当成独立上游额度，也不为没有官方数值 telemetry 的 Provider 编造剩余数值或重置时间。

## 设计

### 事实 Owner

- 配置：`config.HTTP.PublicOrigin` 及 `LLM2API_PUBLIC_ORIGIN` 拥有真实公共 Origin；`LLM2API_HTTP_ADDRESS` 仅拥有监听地址。
- 公共投影：HTTP 文档与 OpenAPI 从 `PublicOrigin` 派生 `/v1`、`/llms.txt`、`/openapi.json`；控制台从 OpenAPI 的 server 读取这些地址，不从浏览器 Origin 推断。
- 协调：Valkey rate/lease bucket 是有效网关容量 owner；新增只读快照和幂等 token 回补操作。
- 凭据健康：registry 的 `health_status`、`cooldown_until` 和 `last_error_kind` 是上游可用性与额度拒绝证据 owner。
- 前端：只展示 OpenAPI、协调快照与 registry 返回的事实，不保存或推断第二套地址、余额或健康事实。

### 数据与控制流

- 启动时加载并验证公共 Origin：只接受 HTTP/HTTPS、带 Host、无用户信息、查询、片段或路径的 Origin。
- 开发脚本以实际 `GatewayPort` 设置监听地址与公共 Origin；非 loopback 或生产部署由部署配置显式提供。
- 控制台读取 OpenAPI server 后才渲染可复制调用配置和 Agent 文档链接。
- `/llms.txt` 与 `/openapi.json` 直接使用同一投影，而非入站 Host。
- requestflow 对成功、Provider usage 已知的尝试回补多预留的 token；凭据容量快照从相同的 HMAC 隔离 Valkey key 读取，不扣减 token。
- 控制 API 读取凭据静态/健康事实后组合容量快照；协调设施不可用时返回“网关容量暂不可观测”，不得把它表示为上游余额用尽。

### 错误与安全

- 无法配置公共 Origin 时启动失败，防止文档生成不可调用地址。
- 控制台读取失败时显示请求错误并禁用复制/跳转，不回退到 `window.location.origin`。
- 公开地址不包含 secret；控制 API 不回显监听内部地址或其他运行配置。
- Key 页将 `healthy`、`cooling`、`repair_required` 与 API Key 的启用状态分开显示；`quota` 错误仅表示上游拒绝且具体余额未知，除非未来接入官方数值 telemetry。

## 生产级切片

- [x] 公共 Gateway Origin 配置、启动验证和开发脚本同步。
- [x] 只读地址投影、机器文档和 OpenAPI 使用唯一 owner。
- [x] 协调容量快照和 Provider-usage token 回补具备幂等、失败关闭和恢复边界。
- [x] 控制 API 组合凭据健康与容量投影；控制台接口文档、新 API Key 配置和 Key 列表处理加载/失败。
- [ ] 定向配置、HTTP 合同、协调与前端构建验证；同步 README 运行事实。

## 实施任务

- [x] 完成端口、代理、文档来源和现有调度语义调查。
- [x] 完成四 Provider 官方额度可观测性矩阵与现有差距调查。
- [x] 实现公共 Gateway Origin 的配置 owner。
- [x] 实现 OpenAPI/机器文档投影和开发代理。
- [x] 实现协调 token 回补与凭据容量快照。
- [x] 组合健康与容量投影，重建控制台文档、新 API Key 配置和 Key 列表状态。
- [ ] 运行定向测试、Go 构建、前端类型检查和生产构建。

## 恶劣路径矩阵

| 边界 | 失败状态 | 恢复 owner | 验证证据 |
| --- | --- | --- | --- |
| 公共 Origin 缺失或非法 | 启动拒绝 | 配置 owner 修正后重启 | 配置定向测试 |
| 控制台地址投影读取失败 | 不显示可复制的猜测地址 | API 重试 | 前端构建与真实 HTTP 合同 |
| Gateway 与控制台端口不同 | 全部文档仍给出 Gateway 地址 | 配置派生 | HTTP 文档合同测试 |
| 反向代理 Host 不可信 | 不采用入站 Host 生成公共 API 地址 | 显式公共 Origin | HTTP 文档合同测试 |
| Provider usage 小于预留 | 仅已完成且 usage 为 Provider 的请求回补差额 | execution ID 幂等标识 | coordination/requestflow 测试 |
| usage 未知或流中断 | 不回补，保持保守预留 | 现有 request/attempt 恢复 | requestflow 恶劣路径测试 |
| Valkey 不可用 | 容量快照明确为不可观测，不影响凭据健康事实 | 协调 owner 恢复后重读 | control API/coordination 测试 |

## 验证计划

- 配置：开发默认、显式非默认端口、生产缺失/非法 Origin。
- HTTP：控制地址投影、`/llms.txt`、`/openapi.json` 的 URL 一致性。
- 协调：快照不扣减、已知 usage 的一次回补和重复回补幂等、未知 usage 不回补。
- 前端：类型检查与生产构建；owner 在桌面浏览器确认复制配置、容量和上游证据显示。
- 额度调研：仅使用官方文档与脱敏代码路径，未执行真实 Provider 请求。

## 收口

- 已运行：`go build ./...`、`pnpm.cmd run typecheck`、`pnpm.cmd run build`，均在改动前通过。
- 未验证：真实 Provider 额度接口；不属于当前文档地址切片。
- commit/push/部署：未执行，等待完整实现与 owner 授权。

## 范围扩展：Provider 能力合同（2026-07-26）

### 新增目标

- Provider 目录只保留 Agnes、智谱 GLM、硅基流动和 Kimi；已删除 Gemini 及其他不在范围内 Provider 的实现、catalog、配置、事实文档与测试引用。
- 模型能力矩阵成为唯一事实：模型实际可用的 chat、stream、tools、tool_choice、parallel tool calls、流式 tool calls、thinking/reasoning、vision、structured output 和限制由经过验证的 Provider 定义/元数据提供。
- `/v1/models` 公开稳定、机器可读的 capabilities；`/v1/chat/completions` 按模型而不是模型名称前缀校验并保真转发。上游不支持时返回有 `model`、`provider`、`capability` 和原因的结构化 4xx。

### 现有事实与边界

- 当前 canonical 和 adapter 已有工具、流式工具调用、图片、reasoning 与 JSON output 的部分内部语义，但 `GET /v1/models` 只投影 OpenAI 最小模型字段，部分 public decoder 仍只接受窄字段集合。
- 旧 Provider 的 catalog、预设、测试、事实文档与真实 Provider 测试脚本已断裂式删除，不保留兼容别名、失效 catalog 记录或迁移桥接。
- `/v1/models` 已有持久模型能力对象；它将扩展为公共 API DTO 的唯一输入，不能在 handler 里按 `agnes-*`/`glm-*` 名称判断。
- 不读取 `.env`、不调用真实 Provider、也不将上游未文档化余额或能力标为已支持。

### 执行顺序

- [x] 调研 Agnes、智谱、硅基流动与 Kimi 的官方模型/API 合同，核验现有 adapter 的每项能力与 wire 差距。
- [x] 删除旧 Provider 的 catalog、adapter、测试、脚本和事实文档；重建四 Provider 的 capability matrix 与持久模型基线。
- [x] 扩展公开 `/v1/models` 合同与 OpenAPI，使其返回稳定 capabilities/limits。
- [x] 由统一 capability validator 处理请求字段与结构化不支持错误；各 adapter 只负责各自 wire 映射。
- [ ] 补齐各 Provider 的静态 wire、流式工具调用、reasoning、图片/结构化输出（若其官方合同支持）和公共 4xx 合同测试。
- [ ] 完成 Gateway 地址、容量快照和已知 usage 回补切片，并运行定向 Go/前端验证。

## 执行清单：保真能力与额度利用（2026-07-26）

### 0. 事实与边界

- [x] 核验三家现有 Provider 的官方能力合同；未证实的能力默认不声明、不转发。
- [x] 核验 Kimi 官方协议、模型发现、余额端点和账户级并发/RPM/TPM/TPD 限速事实。
- [ ] 从 Kimi 官方 `/v1/models` 授权结果或明确官方模型 ID 确认可调用的免费聊天模型；在未确认前不把任一付费模型伪标为免费。

### 1. 能力合同与公共协议

- [x] 以 Provider/上游模型 profile 建立唯一 capability matrix，移除模型名前缀和泛 Provider 推断。
- [x] `GET /v1/models` 公开稳定的 chat、stream、工具、tool choice、严格 schema、并行/流式工具、reasoning、图像、结构化输出和 token 限制。
- [x] 扩展 capability matrix 与公共请求为每个实际模型可验证的高级参数/限制（例如 Kimi `thinking.keep`/Partial Mode/多模态与 SiliconFlow `thinking_budget`、`n`、`top_k`），并将每一个已声明字段无损映射到其上游 wire。
- [x] 将模型 capability 表投影到控制台模型/资源池工作流，让管理员能在添加上游 API Key 前看见全部支持、限制及证据来源。
- [ ] 补充端到端 HTTP 合同：三家既有 Provider 与 Kimi 的模型发现、非流工具、流式工具、reasoning、视觉/结构化输出和不支持能力 4xx；不以真实 secret 或真实调用替代隔离 wire 测试。

### 2. Kimi Provider

- [x] 实现 `kimi` 独立 adapter、错误分类、模型探测元数据、官方 `api.moonshot.cn/v1` 预设和安全请求 wire；不复用“泛 OpenAI Provider”名义。
- [x] 将每个正式注册的 Kimi 模型的思考/推理档位、保留思考、多步工具、流式工具、图片/视频输入、JSON/Partial Mode 和上下文限制写入同一矩阵。
- [x] 基于官方余额端点接入只读、脱敏账户余额证据；无法读取时状态为 unknown，不把 Key 数量或本地桶伪装成余额。
- [ ] 将账户级 Kimi RPM/TPM/并发/TPD 设为共享 scope，保障所有合法 Key 的调度不超限，并在控制台标明共享关系与最后观测时间。

### 3. 额度利用与可观测性

- [x] 增加 Valkey rate bucket 的只读观测与幂等退款原语。
- [x] 在成功且 authoritative usage 小于预留时，将 token 差额按每次租约 idempotency marker 回补；未知/估算/中断不回补。
- [x] 为每条上游 API Key 投影 key-scoped 网关 RPM/TPM/并发剩余、凭据健康/冷却、额度拒绝证据与观测时间；协调不可用时明确标为 unavailable。
- [ ] 在控制台显示“网关可调度余量”“上游账户/项目共享限制”“上游精确余额/未知”三种不同事实，禁止单一伪余额。
- [ ] 基于额度拒绝、冷却和成功 usage 复核调度：同资源池内优先合格 Key、精确消耗实际 token、不可跨资源池借用额度。

### 3.1 手动获取上游状态

- [x] 在“上游资源池 / 上游 API Key”管理工作流提供管理员手动“获取上游状态”动作；动作需 CSRF、服务端管理员权限、短超时与审计，不读取或回显 secret，不做后台自动刷新。
- [ ] Provider adapter 以厂商正式端点或已返回的官方限流响应头为唯一数据源，返回带 `scope`、`observed_at`、`evidence` 的结构化观测；Kimi 余额与账户级限流、以及其他厂商能够官方读取的事实分别建模。
- [x] 每个上游 API Key 页面同时展示：当前启用/冷却/修复状态、网关 credential 维度的 RPM/TPM/并发、上游官方可获取的账户/项目/Key 观测、最近额度拒绝；未公开或请求失败必须显示 `unknown`/`unavailable` 和原因，不能伪造“已榨干”或“剩余”。
- [x] 为 Provider 能力目录和上游 API Key 容量投影建立前后端 HTTP 合同测试：后端固定 snake_case 输出，前端逐字段映射并验证深层参数/容量字段；不以页面运行时容错掩盖字段漂移。
- [ ] 将共享范围明确投影到资源池：同账户或项目的 Kimi RPM/TPM/并发/TPD 不因导入多条 Key 而相加；仅在管理员填写或官方观测到可验证上限后纳入共享调度桶。
- [ ] 补齐人工获取的成功、未知、429、超时、权限拒绝和 secret 脱敏 HTTP 合同；控制台只投影 API 返回事实，操作完成后显示观测时间和证据来源。

### 4. 交付验收

- [ ] 完成三 Provider + Kimi 的 catalog/adapter/公共合同/额度单测与短集成测试；覆盖重试、重复回补、断流和 coordinator 不可用。
- [ ] 完成前端类型检查和生产构建；人工确认桌面控制台的模型能力表、Key 容量状态、错误和空状态不重算事实。
- [ ] 更新 README、接口文档、真实 Provider 验收脚本和版本事实；`5173` 只描述控制台，客户端 Base URL 仅来自配置的 Gateway public origin。

## 范围扩展：上游 API Key 事实重建（2026-07-26）

### 失败证据

- 管理台曾允许同一上游 secret 被多次导入，形成重复的可调度凭据；批量导入只在单次请求内按明文去重，数据库没有凭据身份唯一约束，跨请求和并发写入均可重复。
- “探测全部 Key”由浏览器分别发起请求、再用本地 Promise 状态汇总；页面顶部的成功/失败数因而可与每条凭据已持久化的探测结果不一致。
- 凭据状态操作复用无目标对象的通用确认弹窗，管理员无法在提交前确认实际要变更的上游 API Key、目标状态和调度影响。

### 最终目标与唯一 Owner

- `registry` 拥有上游 secret 的不可逆 HMAC 身份和全局唯一性；secret 不回显、不记录到审计、不作为普通 SHA-256 值持久化。一个实际 secret 在 Gateway 中最多对应一条凭据，包含已退役记录。
- PostgreSQL 的 `provider_credentials.secret_fingerprint` 唯一约束是并发下最终裁决；服务层在调用上游模型发现前先检查相同身份，避免已知重复 Key 的无意义探测。
- `registry` 拥有一次人工批量模型探测的每条结果与汇总；控制 API 只投影该结果，前端不再并发拼接或重算成功/失败数字。
- 凭据状态变更仍由 `registry` 的状态机拥有；控制台仅以同一 `CredentialOperation` 状态投影目标凭据、目标状态、提交中、错误和完成后的最新事实。

### 断裂式实现清单

- [ ] 新增专用凭据身份 pepper、HMAC 指纹和基线 schema 唯一约束；删除无持久唯一性的导入语义。
- [ ] 重建创建、替换和批量导入链路：一次导入中的重复与既有记录均返回明确 `duplicate` 结果，绝不创建第二条凭据。
- [ ] 重建服务端批量模型探测合同；删除浏览器 `Promise.allSettled` 汇总逻辑。
- [ ] 重建凭据启用、停用与退役操作弹窗为以目标 `CredentialOperation` 为中心的界面。
- [ ] 扩展隔离核心 HTTP 验收：导入 -> 去重 -> 模型发现 -> 批量探测 -> 状态变更 -> 候选资格；验证数据库唯一约束和控制 API 投影一致。
- [ ] 运行 sqlc 生成、定向 Go/前端检查、PowerShell 核心脚本语法检查和 `python .\\start_test.py core` 隔离 HTTP 主旅程。

## 范围扩展：控制台统一性与 Linux Docker 验收（2026-07-26）

### 视觉基线与安全边界

- `ResourcePoolsPage` 是控制台密集表格的视觉基线：表头、内容、状态与操作列按字段语义采用稳定的居中/左对齐规则，圆角、边框、行高和间距只使用共享 token。
- 下游 API 密钥保存认证所需的不可逆 `secret_digest`，同时以现有信封主密钥加密保存可恢复副本；列表只投影前缀与状态。明文只能由密钥拥有者或管理员在受 CSRF 保护的单条“显示并复制”操作中按需解密，绝不写入列表、日志或审计详情。

### 执行清单

- [x] 审计 CSS token、共享 DataTable/Dialog 与所有控制台表格的对齐声明，确认回归是上游 API Key 页的页面元数据而非第二套样式系统。
- [ ] 以资源池页为基线重建上游 API Key 页的整行表格对齐与状态块节奏，保留身份/表单语义所需的左对齐。
- [ ] 补齐生产 Compose 与 Windows 服务的 `LLM2API_PUBLIC_ORIGIN`，使其与公开 Gateway 地址同源。
- [ ] 在 Docker Linux 中运行生产 TLS、双 Gateway、迁移、滚动替换、恢复和灾备验收；从真实失败逐项修复，禁止以 Windows 静态检查代替。
- [ ] 重建下游 API 密钥为“加密可恢复、单条按需显示并复制”：创建、替换和重复查看同一事实，列表与审计始终不含明文。
