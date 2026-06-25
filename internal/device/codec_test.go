package device

import (
	"testing"
	"time"
)

// TestCodecRoundTrip 验证编码后的设备码可被正确解码，且载荷一致。
func TestCodecRoundTrip(t *testing.T) {
	c := NewCodec("test-secret")
	exp := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)

	code, err := c.Encode(codePayload{
		V:            1,
		Gateway:      "https://cloud.example.com",
		PairingToken: "mgpair_abc",
		ExpiresAt:    exp,
	})
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	got, err := c.Decode(code)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if got.PairingToken != "mgpair_abc" || got.Gateway != "https://cloud.example.com" {
		t.Errorf("解码载荷不符: %+v", got)
	}
}

// TestCodecRejectsTamperedSignature 验证篡改任意部分都会导致签名校验失败。
func TestCodecRejectsTamperedSignature(t *testing.T) {
	c := NewCodec("test-secret")
	code, _ := c.Encode(codePayload{V: 1, Gateway: "g", PairingToken: "mgpair_x", ExpiresAt: time.Now()})

	// 篡改最后一个字符（签名段）。
	tampered := code[:len(code)-1]
	if code[len(code)-1] == 'A' {
		tampered += "B"
	} else {
		tampered += "A"
	}

	if _, err := c.Decode(tampered); err == nil {
		t.Error("篡改后的设备码应解码失败")
	}
}

// TestCodecRejectsWrongSecret 验证用不同密钥签发的码无法通过校验。
func TestCodecRejectsWrongSecret(t *testing.T) {
	code, _ := NewCodec("secret-a").Encode(codePayload{V: 1, PairingToken: "mgpair_x", ExpiresAt: time.Now()})
	if _, err := NewCodec("secret-b").Decode(code); err == nil {
		t.Error("不同密钥应导致校验失败")
	}
}

// TestCodecRejectsMalformed 验证结构异常的输入被拒绝。
func TestCodecRejectsMalformed(t *testing.T) {
	c := NewCodec("s")
	for _, bad := range []string{"", "x", "a.b", "a.b.c.d", "wrongprefix.payload.sig"} {
		if _, err := c.Decode(bad); err == nil {
			t.Errorf("应拒绝非法输入: %q", bad)
		}
	}
}
