# MySQL Slow Query Lab

这个实验项目提供了一个可重复的 MySQL 慢查询环境：

- 使用 `docker-compose` 启动启用了慢查询日志的 MySQL 8.0。
- Golang 程序（基于 GORM）一次性写入 100 万条以上的订单数据。
- 内置若干典型的慢查询场景（like 前缀模糊、函数操作索引列、低选择性 OR 条件、ORDER BY 非索引文本列），可直接运行并查看 `EXPLAIN`。

## 先决条件

- Docker & Docker Compose
- Go 1.21+（或兼容版本）

## 启动数据库

```bash
make up
```

MySQL 会暴露在 `127.0.0.1:3307`，默认凭证：`slowuser/slowpass`，数据库名 `slowlab`。慢查询日志写在容器的 `/var/lib/mysql/slow.log`。

## 运行 Golang 程序

```bash
make run
```

程序动作：

1. 自动迁移 `orders` 表结构。
2. 若当前数据量不足，使用 GORM 批量写入 100 万订单（可通过 flags 调整）。
3. 顺序执行一组慢查询示例并打印耗时和 `EXPLAIN` 结果。

常用参数（通过 `ARGS` 传给 Go 程序）：

```bash
make seed ARGS="-orders 1500000 -batch 2000"
```

连接信息也可通过环境变量覆盖（默认见 `internal/db/db.go`）：`MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_DATABASE`、`MYSQL_PARAMS`。

## MySQL 慢查询场景

1. **函数包裹索引列**：`SELECT * FROM orders WHERE DATE(created_at) = '2024-01-01'`，函数包裹时间列无法使用索引。
2. **类型不匹配隐式转换**：`SELECT * FROM orders WHERE phone = 13812345678`，phone 为字符串但使用数字常量，触发隐式转换导致索引失效。
3. **范围查询命中索引**：`created_at BETWEEN '2024-01-01 00:00:00' AND '2024-01-02 00:00:00'`，利用范围条件复用 created_at 索引。
4. **类型匹配命中索引**：`SELECT * FROM orders WHERE phone = '13812345678'`，与列类型一致，索引可直接命中。
5. **索引回表查询**：`SELECT * FROM orders WHERE customer_id = 100`，命中二级索引但仍需回表读取完整行（预设 100 万条热点订单），bookmark lookup 成本高。
6. **覆盖索引查询**：`SELECT customer_id FROM orders WHERE customer_id = 100`，只读索引覆盖的字段，避免回表，可与上一场景对比 `Explain`/`rows`/`Extra`。

> 为了放大差异，程序会在 `created_at = 2024-01-01` 附近插入约 2 千条订单，并额外构造 100 万条 `customer_id = 100` 的热点订单。首次运行时可能需要更多时间来填充这些数据。

### Makefile 快捷命令

- `make up` / `make down`：启动或销毁 MySQL 容器。
- `make seed`：只写入数据（可通过 `ARGS="..."` 传入 Go flag）。
- `make compare-index`：跳过写入，仅运行慢查询（聚焦对比“有索引但回表”示例）。
- `make run`：默认流程（迁移、补数据、执行所有场景）。
- `make clean-cache`：删除本地 `GOCACHE`。

查看慢查询日志示例：

```bash
docker compose logs -f mysql | grep -i 'Query_time'
# 或进入容器
# docker exec -it mysql-slow-query-lab-mysql-1 mysql -uslowuser -pslowpass -e "SHOW VARIABLES LIKE 'slow_query_log_file';"
```

## 清理

```bash
make down
```
这会删除容器和数据卷。
