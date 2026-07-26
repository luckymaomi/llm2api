# LLM2API 产品与系统规格

`spec.md` 是当前产品合同、系统边界、唯一事实 owner 和运行拓扑的规范来源。根目录 `plan.md` 记录断裂式重建的实际完成状态；重建未收口时，以计划中的已验证项判断实现，不能把本规格中的目标合同冒充成已交付事实。

## 1. 产品定位

LLM2API 是服务约 200～300 名受控成员的单实例、封闭式商业多 Provider LLM 网关。管理员在线下完成客户交易，在线维护成员、套餐、订阅和上游资源；成员通过一个 Base URL 和自己的 API 密钥调用被授权模型。

产品只有两个业务核心：

1. 多 Provider 统一服务：在官方合同和服务条款内，最大化合法上游 API Key 的 RPM、TPM、并发和周期额度利用率；通过统一 API、资源池硬资格、同级公平选择、冷却、熔断、有界安全重试和崩溃恢复提供可靠服务。
2. 管理员与成员治理：管理员直接创建和治理成员，发布套餐版本、分配订阅并维护 API 密钥；成员只能读取和操作自己的订阅、API 密钥、请求记录与账号。

当前业务对象固定为：

- 成员；
- 套餐及不可变发布版本；
- 成员订阅；
- 成员 API 密钥；
- 上游资源池及上游 API Key。

Provider 是代码拥有的能力目录，不是管理员安装的业务对象。上游资源的合法写入在事务提交后直接进入实时资格读取，不存在捕获、校验、发布配置的人工状态机。

### 用户与价值

- 管理员：集中维护合法上游资源、套餐、成员服务、API 密钥与运行健康。
- 成员：清楚看到自己的有效订阅、可用模型、API 密钥和请求结果，不接触上游 secret。
- 调用方：通过 OpenAI-compatible SDK 或 HTTP 使用稳定、可解释的公共合同。

收入来自统一接入、可靠运营、受控分发和模型资源整合。采购金额在线下管理；网关只记录请求次数、输入/输出 Token、状态、延迟和上游尝试，不处理价格、金额换算、在线收款、充值、开票、客户售价或毛利结算。

### 不在当前范围

- 公共注册、邀请码、组织/租户/企业隔离和成员自行购买；
- 在线支付、充值、开票、优惠码和自动售卖；
- 消费者 OAuth/Session 账号池、批量注册、接码、养号、绕过风控或突破上游额度；
- Kubernetes、多地域主动高可用和移动端控制台；
- 语音、Embedding、Rerank 等独立公共协议。

“免费”只表示合法上游合同中的免费资源或管理员发放的免费套餐，不承诺上游永久免费、无限量或始终可用。

## 2. 用户主旅程

```text
首次创建管理员
  -> 从只读 Provider 目录选择能力并创建资源池
  -> 逐行粘贴一条或多条上游 API Key，自动探测并保存每把 Key 的真实模型快照
  -> 创建并发布套餐版本
  -> 直接创建成员，保存只显示一次的初始密码
  -> 给成员分配订阅
  -> 创建成员 API 密钥
  -> 通过统一 API 调用
  -> 查看请求、Token 用量和资源健康
  -> 更换/停用/退役 Key，续期/暂停/取消订阅或停用成员
```

- 全新系统通过一次性 setup 创建首位管理员；服务端生成高熵初始密码并只显示一次，成功后 setup 永久关闭。
- 不存在公共注册入口。后续成员由管理员输入邮箱和显示名称直接创建，服务端生成一次性初始密码。
- 新手指引不保存独立进度，只根据资源池、活动上游 API Key、已发布套餐、活动成员订阅和 API 密钥的真实持久状态给出当前唯一下一步。
- API 密钥与上游 API Key 必须始终使用明确名称和上下文；一次性密码、完整 API 密钥和上游 secret 不进入日志、审计或浏览器持久存储。

## 3. 公共 API 与 Provider 合同

- 公共合同为 OpenAI-compatible `GET /v1/models`、`POST /v1/chat/completions` 和 `POST /v1/responses`。任意外部 Agent 只把 `llm2api` 视为唯一 Provider；真实 Provider、资源池、上游 API Key、候选与切换原因只属于网关内部事实，不进入公共模型、错误、响应或文档。
- Chat Completions 与 Responses 支持非流式、流式、工具调用、reasoning 和 usage；无法无损表达的能力在发送前明确拒绝。
- Public API 拥有客户端 wire，Canonical Model 拥有内部语义，Provider Adapter 或明确 policy 拥有厂商差异。
- 当前专用 adapter 为 Agnes、智谱 GLM、硅基流动和 Kimi；上游 wire 差异只保留在各自 adapter 内。
- `GET /v1/models` 只返回当前下游 API 密钥经活动成员、活动订阅和不可变套餐版本授权的模型；`owned_by` 恒为 `llm2api`。每个模型返回稳定、机器可读的 capabilities：Chat Completions、可映射的 Responses、stream、工具/选择/严格 schema/并行或流式工具、thinking/reasoning 配置与内容、图片/视频、JSON 输出、消息扩展、非流/流式 usage、token 限制及 `parameters`。`parameters` 为每项公开采样参数给出支持状态、范围、固定值及与 thinking 相关的条件；请求字段只在目标模型能无损表达时转发，否则在发送前返回带公开 model、固定 provider=`llm2api`、capability 与原因的结构化 4xx。
- 公共成功响应使用网关生成的 ID 和客户端请求的公开模型名；公共上游失败只返回稳定、可恢复的 LLM2API 错误，不回显真实 Provider 的原始错误码或正文。内部 request、attempt、审计和管理员观测仍保留脱敏后的真实路由事实用于排障。
- Provider kind、展示名称、builder、权威合同 URL、快照日期、现场验证日期、模型能力目录与 wire 能力由 `internal/providers` 唯一拥有；每把上游 API Key 当前可见的模型 ID 由成功的 `/models` 探测快照唯一拥有，控制 API、资源池和 UI 只投影这些事实。
- 模型、能力、错误和限额是易变外部事实；修改前依据官方资料和隔离 wire 复核。无法权威查询的余额保持未知，不伪造剩余额度。

### 现场兼容基线

以下是 2026-07-22 已完成的隔离证据，不构成上游永久 SLA：

| Provider | kind | 现场模型 | 已证明合同 |
| --- | --- | --- | --- |
| Agnes | `agnes` | `agnes-2.0-flash` | models、chat、stream、tools、流式工具、thinking、usage、取消与未知边界 |
| 智谱 GLM | `zhipu` | `glm-4.7-flash`、`glm-5.2` | models、chat、stream、tools、reasoning、usage、结构化 quota 与同池接管 |
| 硅基流动 | `siliconflow` | `Qwen/Qwen3.5-9B` | models、chat、stream、tools、reasoning、usage 与标准 SDK |
| Kimi | `kimi` | `kimi-k3` | models、chat、stream、工具与流式工具、thinking、图片/视频、JSON Schema、Partial、缓存键、安全标识、官方余额端点 |

标准客户端现场版本为 OpenAI Go `v3.44.0` 与 Python `openai==2.46.0`。

## 4. 上游资源与路由

### Provider 与资源池

- Provider catalog 是代码内置的 adapter 能力、端点与校验数据源，只在创建资源池时提供平台选项；表单同时投影该 Provider 的已核验模型目录、逐模型能力、限制与官方依据，不建立独立 Provider 业务对象或导航。
- 资源池是明确的上游资格边界。管理员选择一个 Provider，使用中文、英文或混合名称创建资源池；稳定 slug 由服务端自动生成，不作为管理员输入或控制台任务。一个资源池只属于一个 Provider，可以容纳多把上游 API Key；其当前模型集合由活动 Key 的成功探测快照并集派生，不能把 Provider 目录误当作每把 Key 都已授权。
- 创建资源池只持久化经过校验的 Provider 和 HTTPS 端点，不发送上游请求；添加或刷新上游 API Key 时才通过 SSRF-safe transport 调用 `/models`。
- 停用或退役资源池只阻止新请求，历史 request、attempt 和审计保持可解释引用。

### 上游 API Key

- 上游 API Key 属于一个资源池。服务端通过该 Key 的 `/models` 响应保存最新成功的模型快照；同池合格 Key 只会参与自己已声明支持的模型的公平选择。
- 管理员通过唯一的“添加上游 API Key”入口逐行粘贴凭据：一行创建一条，多行批量创建，可使用 `名称,上游 API Key` 或只粘贴 Key。结果逐项返回 created/skipped/rejected，绝不回显 secret；已有 Key 可以替换 secret、编辑名称和限额、轻量探测、深度测试、启用、停用和退役。探测失败保留上一次成功快照。
- 每条上游 API Key 还可由管理员手动获取上游状态。网关容量、上游健康/冷却和上游官方观测是不同事实：前者由 Valkey 协调，健康由 registry 拥有，官方观测必须携带 state、scope、source、observed_at 与可读原因。属于同一官方账号或项目的 Key 引用同一个共享容量范围；该范围唯一持有 RPM、TPM、并发和可选 TPD/UTC 重置时刻。没有正式读取端点或调用失败时为 unknown/unavailable，禁止以本地桶、Key 数量或一次连通性伪造余额、剩余额度或重置时间。
- Kimi 余额端点返回账户级 CNY 可用/代金券/现金余额；Kimi 的 RPM、TPM、并发和 TPD 也受账户累计充值权益约束，多把 Key 不得相加。任何 Kimi “免费模型”资格只由当时授权 Key 的 `/v1/models` 或明确官方公布的模型 ID 证明。
- secret 使用版本化 AEAD 加密保存，只在发送边界按需解密。退役会清除调度资格但保留历史脱敏引用。
- 状态至少区分 active、cooling、disabled、retired。探测记录稳定结果类别、延迟、模型和 Request ID；传输阶段明确区分 `dns_resolution_failed`、`outbound_address_blocked`、`upstream_connection_failed`、`tls_handshake_failed`、`provider_transport_failed` 与 `probe_timeout_or_canceled`，响应不返回上游正文、secret 或敏感 header。

### 调度与隔离

- 请求只在订阅版本授权的资源池内选择候选；资源池不可用、冷却或耗尽时返回可理解状态，不自动跨池消费其他资源。
- 硬过滤顺序为成员/Key/订阅/模型/资源池资格、启停、凭据状态、模型绑定、冷却和本次已尝试凭据；合格 Key 先选择管理员 priority 最小的一组，再按同级 weight 比例选择。单 Key 或账号/项目共享 RPM、TPM、TPD、并发容量不足时，在发送前尝试同池下一把；共享范围已耗尽时，关联 Key 不会因数量增加而获得额外容量。
- admission 只维护系统安全队列和全局容量，不设置成员或下游 API 密钥的业务并发上限；资源池、模型、Provider 和上游 API Key 的真实容量通过 Valkey 跨实例协调。
- 只在已知安全发送边界有限重试。上游副作用未知、响应已提交或流已开始时不得换 Key 或资源池盲目重放。

## 5. 套餐、订阅、Key 与用量

### 套餐版本

- 套餐拥有服务端自动生成的稳定标识，以及管理员维护的自然名称、说明、状态和当前发布版本；内部标识不作为控制台输入。套餐版本只表达明确的模型/资源池路由，不携带 Token 额度、成员限流或并发规则。
- 发布版本一经创建不可修改。编辑套餐会校验完整配置、创建递增的新版本并原子设为当前版本；既有订阅继续引用原版本，不能被后台修改悄悄改变。
- 套餐可以停用或归档；停用阻止新订阅，归档不删除历史版本和已有订阅。
- 每个版本的模型路由显式绑定一个模型和一个资源池，确保一次请求具有唯一池边界。

### 成员订阅

- 管理员把某个已发布套餐版本分配给成员，并设置开始时间、可空到期时间和运营备注。`expires_at = NULL` 表示永久有效；状态为 scheduled、active、suspended、canceled 或 expired，状态与时间共同决定新请求资格。
- 暂停、取消和到期只阻止新请求，不改写已完成请求。
- 同一成员可以有多个订阅；请求接受事务只选择一个授权该模型/资源池的活动订阅，并冻结订阅、套餐版本和资源池事实。管理员天然具备活动路由资格，无需给自己分配订阅。

### 成员与 API 密钥

- 系统只有 administrator 和 member 两种角色。管理员可以创建、编辑、停用和删除成员；删除是保留历史外键的终态，撤销全部会话和新请求资格。
- 唯一管理员不能通过在线成员删除流程被移除；锁定恢复只走受控离线命令。
- 成员和管理员可以创建、重叠更换、删除自己的 API 密钥；管理员还可以为任意活动成员操作。完整密钥只显示一次；删除后不再出现在活动列表，但历史请求保留脱敏引用。
- 每把 API 密钥显式绑定一个或多个“模型 + 资源池”路由，不能扩大成员活动订阅的路由资格。

### 请求用量

- 请求不预留或扣减下游 Token 余额。完成、失败、取消和 uncertain 都保存可解释终态；能够获得 usage 时记录输入/输出 Token 及来源，无法获得时明确为未知。
- 套餐、订阅、成员和下游 API 密钥不拥有 RPM、TPM 或并发上限。上游 API Key 可以保存合同或实测容量；Valkey 只拥有可过期协调事实。
- 网关不保存价格版本、币种、成本快照、金额换算或成本汇总。采购和金额属于线下运营事实。

## 6. 失败、恢复与安全不变量

- PostgreSQL 是身份、资源池、上游 API Key、套餐版本、订阅、请求、attempt、usage、mutation 和审计的持久 owner。
- Valkey 只拥有可过期、可重建的限流计数和并发租约；协调不可用时 fail closed。
- 管理会话使用 HttpOnly cookie、独立 CSRF token、过期与撤销；停用/删除成员会阻止后续管理与公共调用。
- 自定义网络事实不由浏览器输入。连接和重定向必须防御 SSRF、DNS 重绑定与内网访问；开发 Fake-IP 放宽不进入生产默认值。
- 请求体、响应字节、队列、并发、连接、流时长和超时有显式上限。普通日志、错误、指标和审计不保存 secret 或请求/响应正文。
- 客户端取消、断连、partial stream、429/5xx、存储/协调失败、进程强杀和重启必须留下可解释终态。未知上游副作用保持 uncertain，不自动重放。
- 流输出后失败不能拼接第二个响应；恢复、取消、回调和清理幂等，execution fencing 阻止过期执行者覆盖新 owner。

## 7. 技术架构

| 层 | 选择 | 职责 |
| --- | --- | --- |
| 服务端 | Go 1.26 模块化单体 | 公共 API、控制 API、恢复 worker 和生产前端 |
| HTTP | `net/http` + `chi` | 请求上限、取消、流式生命周期和中间件 |
| 持久化 | PostgreSQL 18 | 权威持久事实 |
| SQL | `pgx` + `sqlc` + 唯一基线 | 显式事务和确定性生成类型 |
| 协调 | Valkey 9 | RPM/TPM、并发租约和跨实例短期容量 |
| 控制台 | React 19 + TypeScript + Vite 8 | 随 Go 二进制交付的桌面管理端 |
| 前端状态 | TanStack Router/Query/Table | 路由、服务端 cache 和密集表格 |
| UI | Radix、Lucide、CSS tokens | 可访问、稳定尺寸的桌面操作面 |
| 观测 | 结构化日志 + Prometheus | Request ID、稳定错误类别、容量和恢复 |

```text
Administrator / Member Browser
              |
       Control API + embedded Web
              |
Client SDK -> Public /v1 API
              -> API Key -> Member -> Subscription -> Plan Version
              -> Model/Resource Pool -> Upstream API Key -> Provider Adapter
              -> PostgreSQL request/usage facts + Valkey coordination
```

模块化单体适合当前规模。只有真实出现独立扩容或发布需求时才拆服务。

### 模块 owner

```text
internal/
  publicapi/       /v1 鉴权、协议响应与流式边界
  protocol/        OpenAI-compatible wire
  canonical/       消息、工具、reasoning、usage 与错误语义
  providers/       Provider catalog、adapter、能力与错误分类
  identity/        成员、角色、会话与成员 API 密钥
  registry/        资源池、模型投影、上游 API Key、健康与冷却
  subscription/    套餐版本、成员订阅与资格
  admission/       本地公平排队
  coordination/    Valkey 原子限流与租约
  routing/         资源池硬资格与同池候选顺序
  usage/           请求接受与 usage 提交
  requestflow/     接受、发送、流式、结算与恢复编排
  execution/       持久 claim 与 fencing
  responses/       后台 Responses 状态机
  security/        AEAD、摘要、SSRF 与网络策略
  controlapi/      控制 HTTP 合同和权限边界
  store/           PostgreSQL/Valkey adapter 与 sqlc 消费者
  operations/      只读运营聚合与新手指引状态
  app/             配置校验、装配和进程生命周期
```

## 8. 控制台信息架构

管理员导航按任务分为：

- 开始：新手指引、仪表盘、运行状态；
- 上游资源：资源池、上游 API Key；
- 成员服务：套餐、订阅、成员、API 密钥、接口文档；
- 运营数据：API 日志；
- 系统：站点设置与账号操作。

成员导航只包含仪表盘、我的订阅、API 密钥、API 日志和账号操作。导航隐藏不代替服务端权限。

- 首屏先显示当前可用性、需要处理的真实问题和唯一下一步；空、加载、失败、冷却、停用、退役和完成状态都有操作出口。
- Provider 平台与模型能力只在资源池表单中投影；上游 API Key 列表提供“全部资源池”与按资源池查看的下拉筛选；成员、订阅、凭据与请求使用可扫描表格。页面 section 不套装饰卡片，卡片不嵌套。
- 表单使用统一 label/control 网格、稳定控件高度和垂直居中容器；状态与操作列居中，文本列左对齐，数字右对齐。宽表格只在自身边界滚动。
- 控制台只支持桌面浏览器，不维护移动导航、媒体布局或移动验收。
- 视觉审美由 owner 验收；自动证据只保护前端构建、HTTP 权限、状态和真实业务结果。

## 9. 容量与交付拓扑

当前已验证容量基线来自 2026-07-22 的 300 名受控用户、约 60 名活跃用户、双 Gateway、PostgreSQL 18 与 Valkey 9 本机档。旧基线证明稳定数据面地基，不自动证明本次重建后的产品主旅程；重建后由 `full` 和按风险运行的 capacity 重新确认。

- 正式主拓扑是单台 Linux 主机 Docker Compose：Caddy 是唯一公开 80/443 入口，两个 Gateway 位于 edge/backend 网络，PostgreSQL 与 Valkey 只在 backend。
- Gateway 使用只读根文件系统、非 root、`cap_drop: ALL`、no-new-privileges 和显式资源/日志上限；生产镜像引用固定 digest。
- production secret 只通过 file source 进入。应用在连接存储前完成配置校验，生产不自动 migration。
- PostgreSQL 加密异地备份、恢复到空环境、RPO/RTO、Windows SCM 和发布供应链细则由部署文档唯一维护。

## 10. 完成不变量

- 当前没有必须保留的正式生产数据；首次发布前的不兼容 schema、API、配置和事件直接压平唯一基线，不维护旧 migration、双读写或兼容壳。
- 每个事实只有一个 owner；handler、UI、adapter 和测试不保存第二套身份、套餐、订阅、用量、调度或健康事实。
- 阶段可以缩小范围，但纳入范围的成功、错误、并发、中断、恢复、安全、可观测性、测试、文档和目标环境验证必须闭环。
- 最终验收连接真实 Go、PostgreSQL、Valkey、生产前端和目标 Provider，以前端构建、真实 HTTP 合同和标准 SDK 覆盖管理员、成员、取消、刷新、重登、强杀与恢复；控制台 UI 由 owner 人工验收。
- LLM2API 使用 MIT License；LGPL/AGPL 或归属不清的参考源码只研究机制和失败经验，不复制到主干。
