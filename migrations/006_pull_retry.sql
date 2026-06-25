-- 006_pull_retry.sql
-- Phase 5：HTTPS Pull 兜底通道与命令重试。
--
-- 边界不变：cloud 只校验白名单 action 并经 WS/Pull 投递 JSON，不执行命令、不拼接 shell。

-- devices 增补 Pull 相关时间点。
-- 注意：last_pull_at 表示"最近一次通过 HTTPS Pull 联系"，与 WS 的 online（进程内瞬时连接）不同。
ALTER TABLE devices ADD COLUMN last_pull_at DATETIME;
ALTER TABLE devices ADD COLUMN last_pull_status_at DATETIME;

-- commands 增补最近一次错误/重试原因（便于排查；非强制）。
ALTER TABLE commands ADD COLUMN last_error TEXT;

-- 说明：commands 既有的 attempts / max_attempts / leased_by / lease_until / expires_at
-- 在 Phase 5 开始真正使用：
--   leased_by    形如 "ws:<instance>" 或 "pull:<request_id>"，标识投递通道；
--   lease_until  领取后的投递租约截止；对 pending 命令则表示"下次可被领取的时间"（重试退避）；
--   attempts     每次领取（投递尝试）+1，达到 max_attempts 后不再重试而标记 timeout。
