package device

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"mgate-cloud/internal/model"
	"mgate-cloud/internal/util"
)

// ── 设备码编解码（Codec）────────────────────────────────────────────────
//
// 设备码格式：mgate1.<base64url(payload-json)>.<base64url(hmac-sha256)>
//
// payload 仅包含连接所需的最小信息与"一次性" pairing token——刻意【不含】
// 任何永久密钥（device token）。这样即便设备码泄露，攻击者最多在有效期内、
// 且未被使用前尝试一次绑定，无法获得长期凭证；绑定成功后设备码立即失效。
// 签名用 HMAC-SHA256(AppSecret) 防篡改：任何对 gateway/token/过期时间的改动都会校验失败。

const codePrefix = "mgate1"

// codePayload 是设备码中 base64 编码的载荷。
type codePayload struct {
	V            int       `json:"v"`
	Gateway      string    `json:"gateway"`
	PairingToken string    `json:"pairing_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// errInvalidCode 表示设备码结构性无效（格式错误/签名不符/无法解析）。
// 统一一个错误，避免按失败原因区分而便利枚举。
var errInvalidCode = errors.New("device: 设备码无效")

// Codec 负责设备码的签名编码与校验解码。
type Codec struct{ secret []byte }

// NewCodec 用 AppSecret 构造 Codec。
func NewCodec(secret string) *Codec { return &Codec{secret: []byte(secret)} }

// Encode 把载荷编码为带签名的设备码字符串。
func (c *Codec) Encode(p codePayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("device: 序列化设备码载荷失败: %w", err)
	}
	body := codePrefix + "." + base64.RawURLEncoding.EncodeToString(raw)
	sig := c.sign(body)
	return body + "." + sig, nil
}

// Decode 校验签名并解析设备码；任何结构性问题都返回 errInvalidCode。
func (c *Codec) Decode(code string) (codePayload, error) {
	parts := strings.Split(code, ".")
	if len(parts) != 3 || parts[0] != codePrefix {
		return codePayload{}, errInvalidCode
	}

	body := parts[0] + "." + parts[1]
	// 先校验签名再解析载荷：恒定时间比较，杜绝计时侧信道与篡改。
	if !util.ConstantTimeEqualString(parts[2], c.sign(body)) {
		return codePayload{}, errInvalidCode
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return codePayload{}, errInvalidCode
	}
	var p codePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return codePayload{}, errInvalidCode
	}
	if p.PairingToken == "" {
		return codePayload{}, errInvalidCode
	}
	return p, nil
}

// sign 计算 body 的 HMAC-SHA256 并以 base64url 返回。
func (c *Codec) sign(body string) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ── 设备码持久化（PairingStore）─────────────────────────────────────────

// errPairingNotFound 为包内错误，表示按哈希查无设备码。
var errPairingNotFound = errors.New("device: 设备码不存在")

// PairingStore 封装 device_pairing_codes 表的持久化操作。
type PairingStore struct{ db *sql.DB }

// NewPairingStore 构造设备码存储。
func NewPairingStore(db *sql.DB) *PairingStore { return &PairingStore{db: db} }

const pairingColumns = `id, device_id, code_hash, gateway_url, expires_at, used_at, created_by_admin_id, created_at`

// Insert 写入一条设备码记录（仅保存 pairing token 哈希）。
func (s *PairingStore) Insert(ctx context.Context, q querier, p model.PairingCode) error {
	const sqlStr = `
		INSERT INTO device_pairing_codes
			(id, device_id, code_hash, gateway_url, expires_at, created_by_admin_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?);`
	if _, err := q.ExecContext(ctx, sqlStr,
		p.ID, p.DeviceID, p.CodeHash, p.GatewayURL, p.ExpiresAt, p.CreatedByAdminID, p.CreatedAt,
	); err != nil {
		return fmt.Errorf("device: 写入设备码失败: %w", err)
	}
	return nil
}

// FindByCodeHash 按 pairing token 哈希查询设备码；查无返回 errPairingNotFound。
func (s *PairingStore) FindByCodeHash(ctx context.Context, q querier, codeHash string) (model.PairingCode, error) {
	sqlStr := `SELECT ` + pairingColumns + ` FROM device_pairing_codes WHERE code_hash = ?;`
	p, err := scanPairing(q.QueryRowContext(ctx, sqlStr, codeHash))
	if errors.Is(err, sql.ErrNoRows) {
		return model.PairingCode{}, errPairingNotFound
	}
	if err != nil {
		return model.PairingCode{}, fmt.Errorf("device: 查询设备码失败: %w", err)
	}
	return p, nil
}

// MarkUsedIfUnused 原子地把"未使用"的设备码标记为已使用。
//
// 这是 enroll 并发竞争的关键闸门：WHERE used_at IS NULL 配合受影响行数，
// 保证同一设备码即使被并发提交，也只有一个事务能把它标记成功（返回 true）。
func (s *PairingStore) MarkUsedIfUnused(ctx context.Context, q querier, codeHash string, now time.Time) (bool, error) {
	const sqlStr = `UPDATE device_pairing_codes SET used_at = ? WHERE code_hash = ? AND used_at IS NULL;`
	res, err := q.ExecContext(ctx, sqlStr, now, codeHash)
	if err != nil {
		return false, fmt.Errorf("device: 标记设备码已用失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("device: 读取受影响行数失败: %w", err)
	}
	return affected == 1, nil
}

// LatestByDevice 返回某设备最近一次生成的设备码；found=false 表示从未生成。
func (s *PairingStore) LatestByDevice(ctx context.Context, q querier, deviceID string) (model.PairingCode, bool, error) {
	sqlStr := `SELECT ` + pairingColumns + ` FROM device_pairing_codes WHERE device_id = ? ORDER BY created_at DESC LIMIT 1;`
	p, err := scanPairing(q.QueryRowContext(ctx, sqlStr, deviceID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.PairingCode{}, false, nil
	}
	if err != nil {
		return model.PairingCode{}, false, fmt.Errorf("device: 查询最近设备码失败: %w", err)
	}
	return p, true, nil
}

// scanPairing 按 pairingColumns 顺序扫描一行设备码。
func scanPairing(row rowScanner) (model.PairingCode, error) {
	var p model.PairingCode
	err := row.Scan(
		&p.ID, &p.DeviceID, &p.CodeHash, &p.GatewayURL, &p.ExpiresAt, &p.UsedAt, &p.CreatedByAdminID, &p.CreatedAt,
	)
	return p, err
}
