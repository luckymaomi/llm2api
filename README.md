# LLM2API

[![Verify](https://github.com/luckymaomi/llm2api/actions/workflows/verify.yml/badge.svg)](https://github.com/luckymaomi/llm2api/actions/workflows/verify.yml)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

LLM2API 是一个把多把上游 API Key 汇成稳定服务入口的开源网关。

可以把每一把上游 Key 理解成一个小水池：每个池子的模型、限流和可用额度都不同。LLM2API 将同一资源池内支持同一模型的小水池汇成一个大水池。请求会选择当前健康且有可用容量的 Key；遇到限流或可恢复故障时，调度器按规则切换、冷却、熔断和有界重试，让服务尽量持续有水，同时充分利用每把 Key 合法可用的额度。

项目面向管理员已经拥有的免费大模型 API 额度，例如智谱 GLM、Agnes 等平台提供的免费额度。是否可用、可用多久和可用多少，以 Provider 的实时返回与服务条款为准。

## 它能做什么

- 对外只暴露 `llm2api` 这一个 Provider，并用一个 OpenAI-compatible Base URL 提供 `/models`、`/chat/completions` 与 `/responses`；外部 Agent 不需要也不能指定内部 Provider、资源池或上游 API Key。
- 在创建资源池时按 Provider 查看已核验模型目录、能力、参数限制与官方依据；添加上游 API Key 后再自动探测该 Key 实际授权的模型，二者不会混淆。
- 每个资源池只在自己的 Provider 和 Key 集合内调度，绝不把请求切换到别的 Provider。
- 通过优先级、权重、限流、冷却、熔断和有界重试，把同一模型的多把 Key 汇成一个更稳定的大水池。
- 管理员维护资源池、上游 API Key、成员、套餐、订阅和下游 API 密钥；成员只看自己的服务、密钥与调用记录。
- 记录请求次数、输入/输出 Token、状态、延迟和上游尝试事实，帮助排障与观察服务状态。
- 提供控制台接口文档、`/llms.txt` Agent 索引和 `/openapi.json`，方便人和 Agent 配置任意 OpenAI-compatible 客户端。

## 快速启动

下面是 Windows 10/11 上最简单的本地启动方式。准备好 Git、Docker Desktop、Python 3.10+、Go 1.26.5+、Node.js 22.12+ 与 pnpm 10.33.0。

```powershell
git clone https://github.com/luckymaomi/llm2api.git
cd llm2api
npm.cmd install --global --ignore-scripts pnpm@10.33.0
python .\start_dev.py --check
python .\start_dev.py
```

启动脚本会检查环境、启动 PostgreSQL 和 Valkey、构建 Gateway 与控制台，并打开 `http://127.0.0.1:5173`。本地 API Base URL 是 `http://127.0.0.1:8080/v1`。

首次使用按控制台的新手引导完成：

1. 初始化第一个管理员账号并保存只显示一次的初始密码。
2. 创建资源池，选择 Provider。
3. 在“上游 API Key”粘贴一把或多把 Key。系统会自动读取模型列表；“测试全部 Key”只做轻量模型探测。
4. 创建套餐并发布模型路由，创建成员和订阅。
5. 为管理员或成员创建下游 API 密钥。
6. 打开左侧“接口文档”，复制 Base URL、示例或 Agent 索引链接。

## Linux 一键部署

准备一台已安装 Docker Engine 和 Compose plugin 的 Linux 服务器，完成域名 A/AAAA 记录解析，并在云服务器安全组和系统防火墙放行 TCP `80`、`443`。不要预先安装 Nginx：LLM2API 自带的 Caddy 负责 HTTPS 和反向代理。

```bash
curl -fsSL https://raw.githubusercontent.com/luckymaomi/llm2api/master/deploy/quick-install-linux.sh | sudo bash
```

安装器会询问域名和证书通知邮箱，自行生成所有服务 secret、固定镜像 digest、启动数据库/缓存/migration/两台 Gateway/Caddy，并输出控制台地址。完整的开始、停止、日志、诊断、备份和故障反馈说明见 [正式部署与恢复](deploy/README.md)。

## 交给 Agent 部署

把下面这段话完整发给你的本地开发 Agent。它会先检查环境和仓库事实，再按仓库已有的部署文档完成部署；不会把任何真实 API Key、密码或私密配置写入仓库、日志或聊天记录。

```text
请在当前目录部署 LLM2API。先阅读 AGENTS.md、README.md、deploy/README.md 和 .env.example，确认操作系统、Docker、网络与持久化目录。不要读取、输出或提交任何真实 API Key、密码和 secret。

如果是 Windows 本地体验环境：运行 `python .\start_dev.py --check`，修复缺失依赖后运行 `python .\start_dev.py`，确认控制台和 /health/ready 可访问。

如果是 Linux 正式环境：严格按照 deploy/README.md 创建 root-only 的配置和 secret 文件，使用固定镜像摘要与域名，执行仓库提供的安装脚本；安装后只验证 /health/live、/health/ready、控制台登录页和 TLS。不要自行开放额外端口，不要使用 latest 镜像，不要在环境变量或命令行中放入 secret。

完成后报告实际执行的命令、服务地址、健康检查结果、未完成项和需要我补充的配置，但不要回显任何 secret。
```

正式 Linux 部署、升级、备份和恢复步骤见 [deploy/README.md](deploy/README.md)。

## 调用接口

在控制台左侧打开“接口文档”即可看到当前账号可调用的模型、Chat Completions、Responses、流式响应以及 cURL、Python、Node.js 示例。客户端的 Provider 固定为 `llm2api`，模型始终先由 `GET /v1/models` 获取。

```bash
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer $LLM2API_API_KEY"
```

```python
import os
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8080/v1",
    api_key=os.environ["LLM2API_API_KEY"],
)
response = client.chat.completions.create(
    model="replace-with-a-model-from-models",
    messages=[{"role": "user", "content": "你好"}],
)
print(response.choices[0].message.content)
```

部署后的 Agent 入口：

- `https://your-domain/llms.txt`
- `https://your-domain/openapi.json`

## 日常使用

- 新增或更换上游 Key 后，模型探测成功才会把新模型快照写入资源池。
- 刷新探测失败会保留上一次成功的模型快照，不会把可用模型误判为消失。
- 深度测试会发起一次最小生成请求，可能消耗 Provider 额度；轻量探测只读取模型列表。
- 在“上游 API Key”对单条 Key 手动点击“获取上游状态”。列表会分别显示网关本地 RPM/TPM/并发余量、健康/冷却和 Provider 官方观测；没有官方读取接口的周期额度会明确显示未知，不会把本地余量伪装成上游余额。
- Kimi 的官方余额观测是账户级，RPM、TPM、并发与 TPD 也按账户权益共享；多把同账户 Key 不会相加。同账户 Key 必须使用同一个共享上游范围，并按 Provider 公布的 UTC 重置时刻配置 TPD；Kimi 模型以该 Key 的 `/v1/models` 返回为准，项目不会把未经官方确认的模型标记为“免费”。
- `/v1/models` 的每个 `capabilities.parameters` 明确列出可接收的采样、`n`、`top_k`、`thinking_budget` 等字段，以及范围、固定值和与 thinking 相关的条件；客户端应以它为准，不应按模型名推断。
- 删除下游 API 密钥会立即使它失效；历史请求记录仍可用于审计和排障。
- 调用失败时从“API 日志”查看请求状态、延迟和上游尝试；从“上游 API Key”查看每把 Key 的最近探测结果。

停止本地开发环境：

```powershell
python .\stop_dev.py
```

需要清空本地开发数据重新开始：

```powershell
python .\reset_dev.py
```

## 验证

日常修改使用相应的定向测试。自动化验证前端构建、真实 HTTP 合同、真实 Provider、容量与发布物；控制台视觉与交互由管理员在桌面浏览器人工验收：

```powershell
python .\start_test.py full
```

Kitty 源码仓库与本仓库并列时，可以在同一隔离真实 Provider 现场追加 Kitty -> LLM2API -> Agnes 验收；流程会在进程内生成下游 API 密钥，并在清理隔离数据库前核对请求、上游分配和 usage，密钥不会写入 Kitty 配置或日志：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\test-provider-real.ps1 -KittyRepository ..\kitty -AgnesPoolOnly
```

## 文档

- [接口文档](http://127.0.0.1:5173/api-docs)：启动后登录控制台查看。
- [产品规范](spec.md)
- [开发与验收规范](dev.md)
- [正式部署与恢复](deploy/README.md)
- [贡献指南](CONTRIBUTING.md)
- [安全报告](SECURITY.md)

## 能力、调度与额度边界

- `GET /v1/models` 是唯一的客户端能力目录，`owned_by` 恒为 `llm2api`。只有当前 API Key 已授权、并且网关能够无损表达的模型能力才会出现在这里；请求中的每一个已声明字段都会被内部逐模型 adapter 转成真实 Provider wire，无法无损表达的字段会在发送前返回带公开 `model`、固定 `provider=llm2api` 与 `capability` 的稳定 4xx，不会静默丢弃。模型的 reasoning 模式/配置/内容、消息 `name`、非流与流式 `usage`、扩展和参数约束都会随能力事实一起持久化和重建。流式 Chat 需要最终 usage 时发送 `stream_options.include_usage=true`。
- 公共成功响应使用网关 ID 和公开模型名，公共上游错误不会回显真实 Provider、资源池、上游 API Key 或上游原始错误正文；这些脱敏路由事实只在管理员内部观测中使用。
- `/v1/responses` 目前承载可映射的文本、函数工具、流式、存储、续接和后台任务；图像、音频、视频、Embedding、Rerank 或其他不能由当前公共协议无损表达的 Responses 形态不会被伪装成已支持。
- 资源池始终隔离。候选上游 API Key 先按管理员配置的 `priority` 选择最小值，同一优先级内再按 `weight` 比例分配；冷却、熔断、429 和容量耗尽不会自动消耗另一个资源池。
- 同一个 Provider 下属于同一官方账号或项目的多把上游 API Key，可配置到同一个“共享上游范围”。范围的 RPM、TPM、并发和可选 TPD 是单一持久化事实，所有关联 Key 在 Valkey 中原子竞争同一份容量；TPD 使用 Valkey 服务器时间和配置的 UTC 时刻固定重置，已完成请求的权威 usage 差额会幂等回补。当前周期内修改 TPD、周期或重置时刻会保守清零到下次重置，避免错误放宽上游上限。在任意一把关联 Key 上修改该范围，会对全部关联 Key 生效。
- 控制台的“网关余量”是单 Key 的本地调度观测，“共享上游额度”是账号/项目范围的本地调度观测，“上游观测”只显示 Provider 官方实际返回的余额或明确的未知/不可用状态。没有官方可读数据时，系统不会从 Key 数量或本地桶余额推测上游余额。

## License

[MIT](LICENSE)
