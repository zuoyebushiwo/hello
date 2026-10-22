# Go 15 天进阶实验室

> 完成这 15 天后，继续学习独立的 [Go 框架实战 15 天](framework15/README.md)：Cobra/Viper、Chi、Gin、GORM、pgx、Prometheus、OpenTelemetry、Watermill、Asynq、GoFrame 和 Kratos。

这是面向资深 Java 工程师的 Go 进阶路线。目标不是再抄一遍语法，而是把 Java 工程里熟悉的分层、并发、事务、扩展点、可观测性和容错能力，逐一映射到 Go 的惯用写法。

本目录是独立 Go module，只使用标准库，不影响上级 `awesomeProject` 的依赖：

```bash
cd /Users/zhangxuejun/GolandProjects/awesomeProject/golab
go build ./...
go test ./...
go test -race ./...
```

## 使用方法

每一天都遵循同一个闭环：

1. 先执行 `go run ./dayXX_*`，观察场景输出和所有 `[OK]` 断言。
2. 再读对应 `main.go`；重点看并发边界、资源所有权和错误传播。
3. 最后修改一个小条件并补测试，例如把 worker 数改成 2、把超时改短、注入一次重复消息。

单章运行：

```bash
go run ./day01_worker_pool
go run ./day15_capstone
```

也可以用统一入口：

```bash
make build
make test
make race
make run DAY=day06_idempotency
make run-all
```

所有 `main` 都通过 `labkit.Lab` 做断言；失败会以退出码 1 结束，因此可以直接接入 CI。

## 15 天路线

| 天数 | 主题 | 真实场景 | Java 心智映射 | 关键文件 |
|---|---|---|---|---|
| Day 01 | 有界 worker pool | 批量同步任务不能无限创建线程/goroutine | `ThreadPoolExecutor`、拒绝策略、Future | `day01_worker_pool/main.go`、`internal/workerpool/pool.go` |
| Day 02 | context 预算 | 上游 400ms 预算向多个下游传播，避免慢调用拖垮请求 | `ThreadLocal`、超时传播、请求取消 | `day02_context/main.go` |
| Day 03 | 小接口与能力组合 | 同一条复制管道支持普通 sink 和可 flush sink | 接口隔离、`instanceof`、组合优于大接口 | `day03_interfaces/main.go` |
| Day 04 | embedding | SQL/HTTP executor 复用基础执行器，但不能误当继承 | 组合、模板方法、方法集 | `day04_embedding/main.go` |
| Day 05 | 错误链与分类 | 数据库、校验、可重试错误的统一分类 | 异常体系、`cause`、错误码 | `day05_errors/main.go` |
| Day 06 | 至少一次投递下的幂等 | broker 重复投递、崩溃重放、并发重复消费 | 去重表、`INSERT ... ON CONFLICT`、事务边界 | `day06_idempotency/main.go` |
| Day 07 | 锁、SetNX 与 CAS | 两个上游同时完成，只能触发一次下游 | 悲观锁、乐观锁、租约、fencing token | `day07_lock_cas/main.go`、`internal/labkit/kv.go` |
| Day 08 | DAG 调度 | 提取、清洗、汇聚、发布组成菱形依赖图 | `CompletableFuture.allOf`、拓扑排序 | `day08_dag/main.go`、`internal/graph/graph.go` |
| Day 09 | 节点状态机 | 失败重试、重试耗尽、下游递归跳过 | 状态机、幂等重跑、终态判定 | `day09_state_machine/main.go`、`internal/fsm/fsm.go` |
| Day 10 | 事务边界 | 多表写入、回滚、快照隔离、批量写入 | Spring 事务、隔离级别、N+1 | `day10_transaction/main.go`、`internal/tx/tx.go` |
| Day 11 | 注册表与安全 DSN | 按方言注册转换器，密码含特殊字符仍能正确连接 | SPI、策略模式、`UriComponentsBuilder` | `day11_registry/main.go`、`internal/registry`、`internal/dsn` |
| Day 12 | 可观测性与优雅关闭 | JSON 日志、Prometheus 指标、HTTP 关闭不丢请求 | Actuator、Micrometer、graceful shutdown | `day12_observability/main.go`、`internal/labkit/metrics.go` |
| Day 13 | 显式依赖装配 | typed key、懒加载、循环依赖、逆序 Close | Spring Bean、`@PreDestroy`、启动回滚 | `day13_di/main.go`、`internal/di/di.go` |
| Day 14 | 弹性治理 | 令牌桶限流、指数退避 + jitter、熔断半开探测 | Resilience4j、Sentinel、Hystrix | `day14_resilience/main.go`、`internal/resilience/resilience.go` |
| Day 15 | 综合数据流 | 重复消息 + 首次失败重试 + DAG + 指标 + 日志 | 把前 14 天合成一个可运行的小系统 | `day15_capstone/main.go` |

## 每日小节

### Day 01：并发原语，ExecutorService 的 Go 版本

场景：24 个同步任务，约束最多 6 个并发；其中一个任务 panic；队列满时提交方必须能超时退出。

要掌握的不是“会起 goroutine”，而是**并发上限也是资源边界**。池的 `SubmitContext` 同时监听任务槽位、调用方 deadline 和池生命周期；`recover` 把单个任务的 panic 隔离在 worker 内。对照裸起 24 个 goroutine，观察峰值并发如何失控。

Java 对照：`ExecutorService` 不是必须被框架创建；Go 里 channel + `WaitGroup` 就能表达有界队列。拒绝策略由调用方通过 `context` 和返回错误决定。

练习：把 worker 数改成 2，观察 `MaxRunning`；再让 `SubmitContext` 的 deadline 短于任务耗时，确认不会永久阻塞。

注意 `Submit` 的参数是 `func(context.Context)`，没有返回值，语义更接近 `Executor.execute(Runnable)`，而不是 `submit(Callable)`。因此测试不能读取“任务返回值”，只能断言副作用（计数器、`Stats`、输出等）；如果业务需要拿到结果或错误，必须显式传 result/error channel，或设计返回结果的 API。Day 01 的 `main_test.go` 同时演示了这两种情况。

需要 `submit(Callable<T>)` 时使用 `SubmitFuture`：

```go
future, err := workerpool.SubmitFuture(pool, submitCtx, func(ctx context.Context) (Report, error) {
	return loadReport(ctx)
})
if err != nil {
	return err
}

getCtx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()
report, err := future.Get(getCtx)
```

`Future.Get(ctx)` 超时只停止等待，不会取消正在执行的任务；任务执行完仍可再次 `Get`。任务返回的业务错误原样带回，任务 panic 则转换成可通过 `errors.Is(err, workerpool.ErrTaskPanic)` 识别的错误，并保留 `PanicError.Stack`。当前 module 保持 Go 1.22 语言基线，而 Go 1.22 不支持泛型方法，因此入口是包级泛型函数 `workerpool.SubmitFuture`，不是 `pool.SubmitFuture` 方法。

### Day 02：context 的超时、取消与请求级数据

场景：父请求预算 400ms，三个下游分别耗时；另一个 goroutine 完全忽略 `ctx.Done()`，演示泄漏。

Go 的 `context` 是取消树，不是业务参数袋子。请求 ID、trace ID 这类横切元数据适合放入 typed key；project、flow、node 等业务字段应显式放进结构体。子 context 的 deadline 只会更短，不会延长父预算。

Java 对照：`ThreadLocal` 的隐式传递在 Go 中变成显式参数；`Future.cancel`/线程中断对应 `ctx.Done()`，但接收方必须主动检查。

练习：把 `labkit.Sleep` 换成 `time.Sleep`，确认泄漏计数立刻出现。

### Day 03：小接口、能力隔离与 nil interface

场景：复制管道只依赖 `Reader`、`Writer`、`Closer`；目标 sink 可选实现 `Flusher`；Write 和 Close 同时失败时两个错误都要保留。

Go 接口应由消费方定义，越小越容易组合。可选能力用类型断言发现，不要把 flush 塞进 God Interface。`errors.Join` 解决“关闭错误覆盖业务错误”的老问题。最后演示一个经典陷阱：装有 nil 指针的接口本身并不等于 nil。

Java 对照：接口隔离原则相同；“`instanceof Flusher`”对应 `f, ok := dst.(Flusher)`；异常抑制对应 `errors.Join`。

练习：给 sink 增加一个 `Sync` 能力，用可选接口接入，不要修改 `Copy` 的签名。

### Day 04：embedding 是组合，不是继承

场景：SQL executor 和 HTTP executor 复用基础执行器的 ID、Run、Validate；随后通过显式组合修复多态行为。

嵌入会提升字段和方法，但不会产生 Java 那种虚方法分派。`BaseExecutor.Run` 调用的仍是 `BaseExecutor.ID`，不是外层覆盖后的方法。需要多态时显式持有依赖或重新实现方法。嵌入接口还可以把实现对象收窄成只读 API。

Java 对照：embedding 更接近“字段组合 + 方法转发”，不是 `extends`。指针接收者与值接收者的方法集不同，决定能否满足接口。

练习：把外层 `ID` 改成字段而不是方法，观察能否满足接口、调用结果如何变化。

### Day 05：错误链，停止用字符串判断错误

场景：同一函数返回 sentinel、validation、retryable、unknown 四类错误；经过 `%w` 包装后，直接类型断言失效。

Go 没有 checked exception。工程化做法是：哨兵错误用 `errors.Is`，结构化错误用 `errors.As`，多个并发失败用 `errors.Join`。错误分类必须驱动重试、告警和 HTTP 状态码，不能靠 `err.Error()` 文案。

Java 对照：异常类型的 `instanceof` 对应 `errors.As`；`cause` 链对应 `Unwrap()`；多异常抑制对应 `errors.Join`。

练习：新增一个带 `RetryAfter` 的错误类型，在分类函数里提取它，并用它决定退避时间。

### Day 06：至少一次投递下的幂等

场景：同一条消息被投递 3 次；副作用成功后进程崩溃再投递；多个消费者并发拿到重复消息。

消息系统通常保证 at-least-once，不会替业务保证 exactly-once。正确顺序是“先 claim，再执行副作用”；claim 必须原子，内存中用 `SetNX`，数据库中用唯一键或 `INSERT ... ON CONFLICT`。处理后还要 mark done，并设计 TTL 与 lease，避免永久占坑。

Java 对照：`@Transactional + 先查后写` 在并发下并不可靠；唯一约束才是最后防线。

练习：把 `SetNX` 换成“先 Get 再 Set”，跑并发场景，观察多个赢家。

### Day 07：锁、锁内重查与 CAS

场景：两个上游几乎同时完成，下游只能被触发一次；锁有租约，可能过期；旧持有者不能继续写。

只在进程内 `sync.Mutex` 不够，因为调度器可能有多个实例。工程组合是：本地锁保护临界区，进入后重新检查共享状态，再用 Redis `SET NX PX` 或数据库 CAS 做跨进程仲裁。长任务要续租，并用 fencing token 拒绝过期持有者。

Java 对照：`synchronized` 是本地锁；`ReentrantLock` 的租约语义不会自动跨进程。乐观锁版本号就是 CAS。

练习：把租约缩短到任务耗时以内，观察 token 校验如何阻止旧持有者提交。

### Day 08：DAG 校验、拓扑排序与并行层

场景：`start -> extract -> clean_a/clean_b -> join -> publish` 的菱形图；同时演示成环和未知节点。

调度前先验证图，而不是运行到一半才发现环。DFS 三色标记负责报环，Kahn 算法生成拓扑序，按层执行可以做到“层间串行、层内并行”。图只负责结构与算法，运行状态应落在持久化状态机中。

Java 对照：一层可以用一个 `WaitGroup` 模拟 `CompletableFuture.allOf`；Go 没有内置 DAG 依赖框架，算法本身就是能力。

练习：增加一条 `clean_a -> publish` 的边，观察层数如何变化；再提交一个自环确认被拒绝。

### Day 09：节点状态机、重试与下游跳过

场景：节点第一次失败后回到 ready 重试；重试耗尽后进入 failed，下游递归进入 skipped。

状态机把“能不能执行”从布尔标志升级为显式状态：pending、ready、running、success、failed、skipped。每次转换校验前置状态，避免重复执行和非法跃迁。失败是否可重试、最大次数、终态聚合都必须可查询。

Java 对照：节点实例不能只靠 `attempt` 识别，生产中常用 `(nodeInstanceID, rerunID, retryTimes)` 三元组。

练习：新增 `Canceled` 终态，并定义它是否让整个流程失败。

### Day 10：事务边界、回滚、快照隔离与批量写入

场景：多表写入中途失败必须回滚；两个事务之间存在快照隔离；事务内能看到自己的未提交写；对比 N+1 与批量写入。

显式事务边界比隐式魔法更容易推理。Go 的 store/db 抽象要明确“谁开始事务、谁提交、谁回滚”。快照隔离说明读取版本和写入版本是两套视图。批量写入要把多次往返收敛成一次操作。

Java 对照：Spring `@Transactional` 的代理边界换成显式 `Begin/Commit/Rollback`；JPA 一级缓存对应事务内可见自己的写。

练习：让第二张表写入失败，确认第一张表已插入的数据也被回滚。

### Day 11：注册表、扩展点与安全 DSN

场景：同一字段按目标方言注册不同转换器；重复注册要失败；不支持的组合返回可识别错误；密码含 `@ : /` 时不能靠字符串拼接。

注册表是 Go 中常见的 SPI 实现：map + 显式注册 + 启动期校验。重复注册静默覆盖会让环境行为不可预测，应尽早失败。DSN 必须用 `net/url`、`url.UserPassword`、`net.JoinHostPort` 结构化构造，并对 query 编码。

Java 对照：`ServiceLoader`/策略模式对应显式 registry；`UriComponentsBuilder` 对应 `url.URL`。

练习：注册一个同名方言，确认启动失败；再给密码加入 `%` 和 `?`，验证往返解析。

### Day 12：结构化日志、指标与优雅关闭

场景：三次成功、一次失败的请求；每条日志带 request_id；拉取 Prometheus 文本；关闭 HTTP server 时不直接杀进程。

日志字段化后才能稳定检索和聚合，`slog` 的 key-value API 比拼接字符串可靠。Counter 与 Histogram 要遵守 Prometheus 命名习惯。优雅关闭的顺序是停止接收新请求、等待在途请求、再取消后台 context。

Java 对照：Actuator/Micrometer 对应独立 `/metrics` handler；Spring graceful shutdown 对应 `srv.Shutdown(ctx)` + `signal.NotifyContext`。

练习：给指标增加 pipeline 标签，确认高基数标签的风险；让 handler 睡 200ms，观察 `Shutdown` 等待。

### Day 13：显式依赖装配与生命周期

场景：`Config -> DB -> Repo -> Service` 懒加载；Close 按构建完成顺序逆序执行；启动失败只释放已经构建成功的 Bean；循环依赖必须报错。

Go 社区通常偏好显式构造函数，但大型应用仍需要一个最小容器管理生命周期。typed key 避免字符串 key 冲突；懒加载要有并发保护；Close 逆序保证依赖先于被依赖者释放；构建失败时不能关闭不存在的资源。

Java 对照：构造注入、`@PreDestroy`、`DisposableBean`、启动失败回滚都能在几十行代码里表达。

练习：增加一个 `Cache` 依赖 `DB`，确认关闭顺序变成 `Cache -> Repo -> DB -> Config`。

### Day 14：限流、退避重试与熔断

场景：burst=3、rate=5/s 的令牌桶；只对临时错误做指数退避 + jitter；熔断器走完 `closed -> open -> half-open -> closed`。

限流保护自己，重试放大流量，熔断保护上游。三者必须一起设计：只重试可重试错误，退避要加 jitter 防止惊群，熔断打开后要快速失败，半开状态只允许少量探测。令牌桶的时钟要可注入，才能稳定测试。

Java 对照：Resilience4j Retry/CircuitBreaker、Sentinel、Guava RateLimiter 都能在标准库里找到等价骨架。

练习：把 `ShouldRetry` 改成只重试 `DeadlineExceeded`，观察永久错误只执行一次。

### Day 15：综合演练，一条完整的数据流

场景：`start -> extract -> clean_a/clean_b -> join -> publish`。`start` 被 broker 重复投递两次，`clean_b` 第一次失败后重试成功，其余节点正常。

这个演练把 14 天串起来：有界 worker pool 控制并发；context 控制整体预算；DAG 决定依赖；FSM 管状态与重试；`SetNX` 同时处理消息幂等与下游触发守卫；结构化日志和 Prometheus 指标提供可观测性。最终断言六个节点各成功一次、`clean_b` 执行两次、`join` 只触发一次、重复 `start` 被拦截。

重点观察两个不同的幂等键：

- `exec:<node>:<attempt>`：同一条消息只能执行一次。
- `enqueue:<downstream>`：同一个下游只能被上游触发一次。

练习：把 `FailAttempts` 改成 2，确认 `clean_b` 执行三次；再让 `clean_a` 永久失败，确认 `join` 和 `publish` 进入 skipped，流程终态变为 failed。

## Java -> Go 心智迁移清单

| Java 习惯 | Go 中的对应做法 | 需要警惕 |
|---|---|---|
| `ThreadPoolExecutor` | 有界 channel + worker goroutine + `WaitGroup` | 不要每个请求裸起 goroutine |
| `Future.get(timeout)` | `context.WithTimeout` + 结果 channel | `context` 必须由阻塞点主动检查 |
| 异常分类/`instanceof` | sentinel + `errors.Is`/`errors.As` | 不要匹配错误字符串 |
| Spring Bean 单例 | 显式构造函数或小型 typed-key 容器 | 不要用全局可变状态代替依赖注入 |
| `@Transactional` | 显式 `Begin/Commit/Rollback` | 事务对象不能跨 goroutine 共享 |
| `synchronized` | `sync.Mutex`/`RWMutex` | 本地锁不能解决多实例竞争 |
| 乐观锁版本号 | CAS/`SetNX` + 重查 | 别忘了过期租约与 fencing token |
| Actuator/Micrometer | `slog` + `/metrics` + `http.Server.Shutdown` | 日志不要拼字符串，metrics 不要高基数 |
| Resilience4j | 令牌桶 + backoff + breaker | 不要重试所有错误，不要无 jitter 退避 |

## 从原工程提炼出的反向清单

学习原 `dap-dataflow` 的架构时，也要区分“业务上值得借鉴”和“Go 上不应复制”的部分：

- 不要恢复 God Interface。按消费方需要拆成小接口。
- 不要在库代码里 `log.Fatalf`、`os.Exit`。返回错误，由 `main` 决定退出。
- 不要用 `panic` 做普通错误处理。panic 只用于不可恢复的编程错误，并在 goroutine 边界隔离。
- 不要用无限 goroutine 模拟线程池。并发必须有上限、队列、超时和拒绝策略。
- 不要用字符串拼接 SQL、DSN 或 JSON。使用参数化查询和结构化编码。
- 不要把 `context` 当业务参数容器。只放取消、deadline 和横切元数据。
- 不要把进程内锁当分布式锁。多实例场景必须使用数据库/Redis 等共享仲裁机制。

## 目录约定

```text
golab/
├── day01_* ... day15_*/   每日可执行场景与断言
├── internal/labkit/       输出、KV、队列、metrics 等教学基础设施
├── internal/workerpool/   有界并发池
├── internal/graph/        DAG 校验、拓扑排序、分层
├── internal/fsm/          节点状态机
├── internal/tx/           事务与快照演示
├── internal/dsn/          DSN 安全构造
├── internal/registry/     方言转换器注册表
├── internal/resilience/   限流、重试、熔断
└── internal/di/           依赖装配与生命周期
```

每章 `main.go` 是可执行教程，`internal` 包是可复用实现，测试负责锁住边界行为。建议按天完成，但不必按天孤立阅读：Day 15 是前 14 天组件的组合验收。
