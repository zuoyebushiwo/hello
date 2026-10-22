# Go 框架实战 15 天

这是 `golab` 之后的第二阶段，不再补 Go 语法，而是围绕真实开源库和框架完成 15 个可运行、可测试的工程切片。全部示例只使用 Go，不包含 Java 对照。

这是一个独立 Go module，第三方依赖不会污染只用标准库的 `golab`：

运行环境为 Go 1.27+；其中 OpenTelemetry SDK 当前版本要求 Go 1.26+。

```bash
cd /Users/zhangxuejun/GolandProjects/awesomeProject/golab/framework15
go test ./...
go test -race ./...
```

每章都有 `main.go` 和 `main_test.go`。每节 `main.go` 开头直接打印框架简介，包括它是什么、解决什么问题、社区采用情况、适用场景、慎用边界和替代方案；先运行 `main` 建立选型判断，再读后面的代码和测试。

## 15 天路线

| 天数 | 框架/库 | 场景 | 测试重点 |
|---|---|---|---|
| Day 16 | Cobra + Viper | CLI、配置分层、启动校验 | 默认值、文件、环境变量、flag 的优先级 |
| Day 17 | Chi | 贴近 `net/http` 的路由和中间件 | 请求 ID、业务 404、超时、panic 隔离 |
| Day 18 | Gin | JSON 绑定、参数校验、Recovery | 合法请求、多字段错误、非法 JSON |
| Day 19 | GORM + SQLite | 转账事务、幂等键、乐观锁 | 双边提交、重复请求、余额不足回滚 |
| Day 20 | pgx + pgxmock | 批量查询、事务、`CopyFrom` | 避免 N+1、Commit/Rollback、批量写入 |
| Day 21 | 标准库 + validator | API 错误契约、严格 JSON、版本化 DTO | `problem+json`、未知字段、v1/v2 兼容 |
| Day 22 | backoff + singleflight + ants | 重试、请求合并、有界 goroutine | 临时/永久错误、缓存击穿、并发峰值 |
| Day 23 | slog + Prometheus + OpenTelemetry | 日志、指标、trace | route 基数、状态码、span 生成 |
| Day 24 | golang-lru + singleflight | 本地缓存、TTL、缓存击穿 | LRU 淘汰、过期、并发只回源一次 |
| Day 25 | Watermill | 事件总线、fan-out | 多消费者、消息契约、至少一次投递 |
| Day 26 | Asynq + miniredis | Redis 异步任务队列 | 入队、worker 消费、并发处理 |
| Day 27 | GoFrame | 最接近 Spring Boot 的全家桶 | 路由、中间件、`v` tag 校验、状态码 |
| Day 28 | Kratos | Proto-first 微服务 transport | Context JSON、Filter、业务错误 |
| Day 29 | JSON Schema | 消费者契约测试 | 响应校验、严格 schema、版本破坏 |
| Day 30 | Chi + GORM + OTel + Prometheus | 综合转账服务 | 幂等、原子性、指标、trace、端到端 |

## 运行

单章运行：

```bash
go run ./day16_cobra_viper
go run ./day30_capstone
```

统一入口：

```bash
make build
make test
make race
make run DAY=day23_observability
make run-all
```

## 选型结论

GitHub 数据查看于 2026-10-22，star 只代表热度，不等同于生产采用率。

| 方向 | 推荐 | 约 star | 采用判断 |
|---|---|---:|---|
| 最像 Spring Boot | GoFrame | 13k | 国内业务系统常见，全球不是主流 |
| 主流 Web API | Gin | 89k | 全球使用最广的 Go Web 框架 |
| 标准库风格 | Chi | 23k | 中间件和 `net/http` 生态最兼容 |
| 成熟 Web 框架 | Echo | 33k | 全球使用广泛，API 完整 |
| 高性能 HTTP | Fiber | 40k | 热度高，但基于 fasthttp，不能直接复用 `net/http` 中间件 |
| 国内微服务 | Kratos | 26k | B 站开源，proto-first 项目常用 |
| 快速微服务生成 | go-zero | 33k | 国内采用广，框架绑定较强 |
| 老牌 MVC | Beego | 32k | 存量项目多，新项目不建议首选 |
| 分布式运行时 | Dapr | 26k | CNCF 项目，适合已有基础设施的团队 |

工程库：

| 能力 | 推荐 | 说明 |
|---|---|---|
| 命令行 | Cobra | Kubernetes、Hugo 等项目使用 |
| 配置 | Viper | YAML、环境变量、flag 统一解析 |
| ORM | GORM | 最主流；复杂 SQL 仍应手写 |
| PostgreSQL | pgx | 生产首选驱动；批量导入用 `CopyFrom` |
| 校验 | go-playground/validator | 事实标准，Gin 也内置它 |
| 测试断言 | Testify | `require`/`assert` 最常用 |
| 日志 | `log/slog`、zap、zerolog | 新项目优先标准库 `slog` |
| 指标 | Prometheus client_golang | 与 Prometheus 生态直接对接 |
| 链路追踪 | OpenTelemetry Go | 跨服务 trace 的 CNCF 标准 |
| 缓存 | golang-lru、ristretto、bigcache | 按命中率、内存、淘汰策略选择 |
| 事件总线 | Watermill | Kafka/RabbitMQ 可替换的抽象层 |
| 任务队列 | Asynq | Redis 驱动，适合异步任务和定时任务 |

## Apache Commons 的 Go 对应

| Commons | Go 对应 | 建议 |
|---|---|---|
| `commons-lang3` | `strings`、`slices`、`maps`、`strconv`；[`samber/lo`](https://github.com/samber/lo) | 先用标准库；`lo` 只在确实减少样板代码时使用 |
| `commons-collections4` | 标准库 map/slice；`container/list`；`gods` | Go 没有统一集合库，不要为了“像 Java”引入大型集合抽象 |
| `commons-io` | `io`、`os`、`io/fs`；[`afero`](https://github.com/spf13/afero) | 文件系统抽象和测试可用 afero |
| `commons-codec` | `encoding/base64`、`encoding/hex`、`crypto/*` | 直接用标准库 |
| `commons-compress` | `archive/tar`、`archive/zip`、`compress/gzip` | 直接用标准库 |
| `commons-csv` | `encoding/csv` | 直接用标准库 |
| `commons-pool2` | `sync.Pool`；[`ants`](https://github.com/panjf2000/ants) | 普通对象复用优先 `sync.Pool`，goroutine 池用 ants |
| `commons-validator` | `go-playground/validator` | 入口校验 + 领域不变量双层保护 |

## 使用边界

- 不要为了“全家桶”同时引入 Gin、Chi、GoFrame。一个服务只选一个 HTTP 内核。
- GoFrame 适合想要开箱即用体验的团队；Chi/Gin + 自选库适合希望控制依赖和生命周期的团队。
- Kratos/go-zero 的收益在多服务、proto、注册中心、治理链路；单体服务引入它们通常过重。
- GORM 适合常规 CRUD；复杂报表、批量写入和热点 SQL 应使用 `pgx` 或手写 SQL。
- Asynq 示例使用 miniredis 便于离线运行；重试、归档和 Redis 故障恢复应在真实 Redis 或 Testcontainers 中做集成测试。
- Day 20 使用 `pgxmock` 验证 SQL 契约；生产代码使用 `pgxpool.New(ctx, os.Getenv("DATABASE_URL"))`。
- 不要把 trace ID、user ID 等高基数标签放进 Prometheus label；使用日志或 trace 关联。

## 建议学习顺序

先把 Day 16–18 跑通，建立 CLI、配置、HTTP 路由和中间件的基本盘；再学 Day 19–21 的数据与 API 契约；Day 22–26 学习弹性、可观测性、缓存、事件和队列；最后用 Day 27–30 比较全家桶、微服务底盘、契约测试和综合服务。

这套路线对应的不是“会调用 15 个库”，而是能判断：什么时候该用标准库，什么时候值得引入框架，以及框架失败时边界在哪里。
