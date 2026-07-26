# LLM2API 与 Kitty 流式输出及迁移回滚修复

## 需求文档

沿 Kitty 请求构造、LLM2API 公共模型能力、canonical 请求、Agnes adapter、SSE flush、Kitty 消费和宿主渲染验证流式输出。普通文本和带工具定义的 Agent 回合都必须在完成前产生多个增量片段；工具回合必须保持 `stream:true` 并完成工具调用及后续回复。同步修复 PostgreSQL baseline migration 的 Down 外键顺序，并完成真实 Provider、取消、错误、终止和脱敏验收。

## 当前事实

- 两个仓库当前均基于远端 `master` 且初始工作区干净；本任务按 owner 最新指令继续直接在 `master` 工作。
- Kitty `src/provider/request.ts` 在存在工具且 `supportsStreamingTools=false` 时使用非流式 adapter。
- Kitty `src/agent/turn/run.ts` 从 LLM2API `/v1/models` 动态读取 `tools.streaming_tool_calls`。
- LLM2API `/v1/models` 由 `internal/publicapi/models.go` 投影 registry 能力；Agnes catalog 的 `LiveCapabilities` 已记录 `streaming_tools`，但 `internal/providers/agnes.go` 的能力值缺少 `ToolStreaming`，导致持久/公共投影为 false。
- `migrations/00001_baseline.sql` Down 删除 `providers` 前未删除依赖它的 `upstream_capacity_scopes`。
- 本机存在隔离真实 Provider 验收脚本和 `key.txt`；密钥只允许由脚本进程注入，不能输出、记录或提交。

## 失败证据

- 静态 owner 对照显示 Agnes 的外部能力目录声明 `streaming_tools`，而 adapter capabilities 没有对应 `ToolStreaming=true`。
- Kitty 的唯一流式选择条件明确依赖该能力，因而工具回合会主动发送 `stream:false`。
- baseline Down 的删除顺序违反 `upstream_capacity_scopes.provider_id -> providers.id` 外键依赖，真实 `goose reset` 会在删除 providers 时失败。

## 最终目标

1. Agnes 能力目录、持久模型能力、`/v1/models` 和 Kitty 动态能力均声明并消费流式工具调用支持。
2. 真实 Kitty -> LLM2API -> Agnes 普通文本与工具 Agent 回合观察到完成前的多片段时间证据，且后续工具回复完成。
3. 取消、上游错误、SSE 终止、敏感信息脱敏均有真实证据；不通过显示层造假流式或关闭工具绕过。
4. migration `up -> reset/down -> up` 在真实 PostgreSQL 隔离容器中通过。
5. 相关短测、类型检查、构建、仓库规定验收通过后，两个仓库在 `master` 提交并推送；Kitty 按现有版本策略发布 npm patch，并在全新临时目录安装验证。

## 设计与 owner

- Provider 能力唯一 owner：LLM2API `internal/providers` catalog；registry/publicapi 只投影，不重算。
- Kitty 请求流式选择唯一 owner：动态 LLM2API 能力投影进入 `ProviderCapabilities`，`request.ts` 只消费它。
- SSE 分片与终止由 LLM2API requestflow/publicapi 流式管线和 Provider adapter 负责；Kitty 只消费真实增量。
- migration 依赖顺序由 baseline SQL Down 负责；不增加兼容迁移。

## 实施任务

- [x] 完成两仓库约束、Git、请求链和 migration owner 调查
- [x] 以真实 Agnes/Kitty relay 与 LLM2API `/v1/models` 证据确认流式工具能力
- [x] 修正 Agnes `ToolStreaming` 能力及对应合同、测试和文档
- [x] 补齐 Zhipu `glm-4.7-flash` 精确模型 profile；Kitty 以 LLM2API 预设为默认 Provider
- [x] 修正 baseline Down 外键删除顺序
- [x] 运行定向测试、类型检查和构建
- [x] 运行真实 Kitty -> LLM2API -> Agnes 流式、工具、取消、错误、终止和脱敏验收
- [x] 运行 migration up -> reset/down -> up
- [x] 修复 operations production 验收缺少 `LLM2API_PUBLIC_ORIGIN` 导致的网关启动失败，并增加脱敏启动诊断
- [ ] 提交、推送、观察 CI；Kitty npm 发布暂按 owner 最新指令不执行

## 恶劣路径与恢复

- 客户端取消或断连：requestflow 保留已接受/部分流事实，不盲目重放；Kitty abort 只结束当前消费。
- 上游 429/5xx 或畸形流：按 canonical error kind 进入冷却/终态，SSE 以终止事件结束，不伪造完成。
- 日志、响应、审计和 npm 包不包含 Provider secret、下游 API key 或上游 API key；验收脚本退出时清理隔离状态。

## 验证计划

- LLM2API：`go test ./internal/providers ./internal/registry ./internal/publicapi ./internal/requestflow`、`go build ./...`、`git diff --check`。
- Kitty：provider 定向测试、`npm.cmd run verify`、`git diff --check`。
- 真实验收：`powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-provider-real.ps1 -KittyRepository ..\kitty -AgnesPoolOnly`，另用 Kitty/网关 HTTP 记录分片到达时间和请求体 `stream` 字段；不输出任何密钥。
- migration：`powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-migrations.ps1`，并核对 up/reset/down/up 的 PostgreSQL 外键顺序。

## 收口

完成后只记录实际根因、改动文件、真实时间/分片证据、命令结果、两个提交 SHA、推送分支、CI 链接/状态、npm 包名/版本和安装结果；未验证项与用户此前暴露密钥需轮换的风险必须明确列出。

## 验证记录

- `go test ./...`、`go build ./...`：通过。
- Kitty `npm.cmd run verify`：459 项，458 通过、1 跳过、0 失败。
- `scripts/test-migrations.ps1`：真实 PostgreSQL up -> reset/down -> up 通过。
- `scripts/test-provider-real.ps1 -KittyRepository ..\kitty -AgnesPoolOnly`：真实 Kitty relay 通过，Kitty eval 报告 `streamingTools=true`、流式 smoke、生产 turn、上下文压力、后台、浏览器、工具修复均通过；账本 `requests=66`、`distinctUpstreamKeys=5`、`authoritativeTokens=529849`。
- 本机 Zhipu 候选 `/models` 均返回 401，未将 `glm-4.7-flash` 宣称为本次真实 Zhipu 成功调用；其精确 profile 已加入代码目录供动态探测匹配。
- `scripts/test-operations.ps1`：通过；此前首个失败根因为 production profile 未设置合法 `LLM2API_PUBLIC_ORIGIN`，网关按配置校验退出。
- `go vet ./...`：通过。
