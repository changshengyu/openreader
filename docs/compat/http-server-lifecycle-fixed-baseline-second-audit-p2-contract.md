# HTTP 服务生命周期固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前实现基线：`OpenReader@5155b40`  
审查日期：2026-08-25  
状态：**inventory-complete / implementation-pending**

## 1. 范围

本轮只覆盖单容器 Go HTTP 进程的协议入口和停止生命周期：

- 请求头读取时限与总大小；
- `SIGINT`/`SIGTERM` 后停止接收新连接、有界等待普通在途请求并退出；
- WebSocket、章节缓存 SSE、书源调试 SSE 的停止行为；
- scheduler、自动备份和导入 stage cleanup 的停止通知；
- 监听失败、正常停止和强制停止的日志/退出语义。

各 route 的 body、认证、事务、远程抓取、文件、备份和 parser 合同不重开。本轮不改变 API 路径、响应
schema、前端状态、SQLite schema、持久目录或备份格式。

## 2. 固定上游与部署合同

固定上游 `server/bin/startup.sh` 显式传入 `--server.max-http-header-size=524288`；
`server/bin/shutdown.sh` 通过 `kill <pid>` 发送标准终止信号。服务由 Spring Boot 2.1.6 和 Vert.x 3.8.1
承载，固定上游没有为 Reader、SSE 或 WebSocket 设置短全局响应写时限。

OpenReader 改为一个 PID 1 Go binary，`Dockerfile` 以 `CMD ["/app/openreader"]` 直接启动，Compose 使用
`restart: unless-stopped`。因此接管 `SIGTERM`、限制未完成请求头并保留长连接是 Go/容器运行时的等价
适配，不是新增产品功能。Docker 普通 stop 的默认宽限通常短于长远程任务；服务自己的停止预算必须
在外层杀进程前结束。

## 3. 当前差异与裁决

| 边界 | `OpenReader@5155b40` | 裁决 |
|---|---|---|
| 请求头大小 | `router.Run` 使用 `net/http` 默认 `MaxHeaderBytes`（1 MiB），宽于固定上游 512 KiB。 | **must-fix**：显式恢复 512 KiB。 |
| 请求头时限 | 未设置 `ReadHeaderTimeout`，客户端可无限慢速提交 header 并占用连接。 | **must-fix / security adaptation**：只限制 header 完成时间。 |
| 普通 body/response | route 已各自拥有 actual-read body 和远程工作预算；存在大文件上传、备份和长正文缓存。 | **must-preserve**：不得设置全局 `ReadTimeout` 或 `WriteTimeout`。 |
| 停止信号 | `main` 直接调用 `router.Run`；Go 默认收到 `SIGINT`/`SIGTERM` 后立即退出，不返回 main，也不执行 defer。 | **must-fix**：显式接管信号并调用 `http.Server.Shutdown`。 |
| 后台服务 | cleanup context、scheduler 和 backup 仅由 defer cancel/Stop；当前信号路径跳过这些 defer。 | **must-fix**：正常停止路径必须调用；本轮不宣称已开始的历史定时任务可无限等待。 |
| SSE/WebSocket | SSE 绑定 request context；WebSocket 已 hijack，不受 `Shutdown` 自动管理。 | **must-fix**：停止不得被永久长连接卡死；WebSocket hub 必须可关闭，SSE 最迟在总预算到期时取消。 |

静态反例已经充分：`backend/main.go` 唯一监听语句是 `router.Run(cfg.Address)`，仓库不存在
`signal.Notify`、`signal.NotifyContext`、`http.Server.Shutdown` 或 server timeout 配置。由于 Go runtime
对未处理 `SIGTERM` 直接终止进程，当前 `cleanupCancel`、`sched.Stop()` 和 `backupSvc.Stop()` defer 在
容器正常停止时不可达。红测阶段还必须用真实子进程和 TCP socket 固化该反例。

## 4. 目标运行时合同

1. 生产 router 必须由显式 `http.Server` 承载：`Addr=cfg.Address`、`Handler=router`、
   `MaxHeaderBytes=512*1024`、`ReadHeaderTimeout=10s`。不得设置全局 `ReadTimeout`、`WriteTimeout`，也不得
   用它们替代 route body/remote budgets。
2. 监听在独立 goroutine 启动。仅 `http.ErrServerClosed` 是正常停止；端口占用、非法地址和其它 listen
   错误必须返回非零，不得等待信号或误记为正常退出。
3. 进程接收第一次 `SIGINT`/`SIGTERM` 后立即停止接收新连接，并以 8 秒总预算调用
   `http.Server.Shutdown`。预算覆盖 HTTP drain 和随后 cleanup，不得为每个组件重新获得完整 8 秒。
4. shutdown 开始时关闭 WebSocket hub，使已 hijack 的连接收到正常 going-away close 或至少被关闭；
   不再接受新 sync client。普通在途请求可在预算内完成，事务/原子文件发布保持原语义。
5. SSE 不新增 wire event，也不把 shutdown 暴露为业务错误。能在预算内完成的普通请求保持原结果；
   预算到期后必须 `Close` server、取消根 cleanup context，并让 request-context-aware SSE/remote work 停止。
6. 无论监听错误、正常 drain 或超时强制关闭，cleanup cancel、scheduler Stop、backup Stop 必须恰好一次；
   Stop 必须可安全重复调用，不能因 double-close panic。已开始工作仍由现有 SQLite transaction、临时文件
   cleanup 和原子 rename 保证 crash safety，本轮不承诺无限等待第三方远程源。
7. 日志只报告信号类别、正常/超时/监听错误和固定组件名，不打印 JWT、URL query、请求 header/body、
   WebDAV credential、数据库详情或 host path。第二次信号允许立即强制退出。
8. 不新增环境变量。512 KiB、10 秒 header 和 8 秒 shutdown 是固定 runtime 常量，避免部署间产生无法
   复现的长连接/停止差异。

## 5. 测试先行门

1. 在旧实现上新增 server-construction 合同：显式 512 KiB、10 秒 header，且 read/write timeout 为零；
   旧 `router.Run` 路径必须先红。
2. 真实 TCP 测试提交不完整 header，证明连接在 header deadline 后关闭；512 KiB 内正常请求保持，超过
   上限被拒绝且 handler 未运行。测试不得 sleep 10 秒，应允许注入更短测试时限而不改变生产常量。
3. 真实子进程占用监听地址，必须快速非零退出；正常服务收到 SIGTERM 后退出码为零，并出现可判定的
   shutdown/cleanup 证据。旧实现应表现为 signal termination 且无 cleanup 证据。
4. 进程级测试用阻塞普通 handler 证明 signal 后拒绝新连接、已开始请求可在预算内完成；另用超时
   handler 证明预算后强制取消退出，不残留进程。
5. WebSocket 与两路 SSE 分别建立真实连接后触发 shutdown，证明 hub/stream 终止且不等待到测试超时；
   非 shutdown 的缓存 abort、书源 debug、server-only sync 协议保持现有断言。
6. focused、focused race、`go vet ./...`、Go 全量、frontend 全量/build、真实 Go HTTP/SSE/WebSocket、
   本地 candidate `docker stop`/restart、fresh/historical/portable/restart 卷门通过后才可发布 Docker。

## 6. 数据与允许差异

- 不新增 schema、migration、startup scan、目录、route、payload、备份 member、manifest、浏览器 key 或
  前端 UI；现有 `data/cache/library` 不扫描、不移动、不重写。
- 进程被迫退出时继续依赖已签收的 SQLite WAL/transaction、stage cleanup、原子文件 rename 和 mounted
  volume 合同；本轮不能以“优雅停止”掩盖非原子写入。
- 512 KiB header 与固定上游一致；10 秒 header deadline、8 秒有界 drain、显式 WebSocket close 和
  idempotent cleanup 是 Go PID 1 的安全适配。
- 不设置全局 response/write timeout 是兼容要求：章节缓存 SSE、书源调试 SSE、WebSocket 和合法大文件
  工作不能被统一短时限截断。

## 7. 实施顺序

1. 独立提交本合同及 REST/gap/matrix/security inventory，不改应用或测试。
2. 在 `5155b40` 旧实现上提交 server/header/signal/long-connection 红测。
3. 提取窄作用域 server runner、信号源和 shutdown hook；为 hub/Stop 增加最小幂等关闭能力。
4. 完成专项/race/full/runtime/container/卷门；形成可验证切片后提交推送并只在本机发布 amd64/arm64。
