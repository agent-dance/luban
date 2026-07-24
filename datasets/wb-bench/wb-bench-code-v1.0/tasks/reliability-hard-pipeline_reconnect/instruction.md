redis pipeline 遇到连接断开后状态有点乱，有时 watch 还残留，有时命令被重复执行。麻烦把 reconnect 后可安全重放和不能重放的情况分清楚，执行完也要清理 pipeline 状态。
