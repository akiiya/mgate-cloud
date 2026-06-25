-- 001_init.sql
-- 初始化核心表：管理员账户与审计日志。
-- 说明：schema_migrations 表由迁移器在运行时单独创建，不在此处声明，
--       这样迁移版本的记录逻辑与业务表结构解耦，便于演进。

-- 管理员账户表。
-- id 使用应用层生成的随机字符串（而非自增整数），避免顺序可猜测，
-- 也方便后续多实例部署时不依赖数据库自增序列。
CREATE TABLE admins (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    -- 仅存储口令哈希（bcrypt），明文口令永不落库。
    password_hash TEXT NOT NULL,
    -- status 预留账户启用/禁用能力，Phase 1 默认 enabled。
    status        TEXT NOT NULL DEFAULT 'enabled',
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    last_login_at DATETIME
);

-- 审计日志表。
-- 作为安全审计的统一落点，记录"谁、在何时、对什么、做了什么"。
-- metadata_json 存放结构化补充信息，写入前必须脱敏（不含口令/令牌/cookie）。
CREATE TABLE audit_logs (
    id            TEXT PRIMARY KEY,
    actor_type    TEXT NOT NULL,   -- 行为主体类型：admin / system
    actor_id      TEXT,            -- 行为主体 id（系统事件可为空）
    action        TEXT NOT NULL,   -- 事件动作，稳定枚举，如 admin.login.success
    target_type   TEXT,            -- 操作目标类型
    target_id     TEXT,            -- 操作目标 id
    ip            TEXT,
    user_agent    TEXT,
    request_id    TEXT,            -- 关联单次请求，便于串联日志
    summary       TEXT,            -- 人类可读的简短描述
    metadata_json TEXT,            -- 脱敏后的结构化补充信息（JSON）
    created_at    DATETIME NOT NULL
);

-- 审计查询通常按时间倒序浏览，建立时间索引。
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
-- 按行为主体过滤（例如查看某管理员的全部操作）。
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_type, actor_id);
