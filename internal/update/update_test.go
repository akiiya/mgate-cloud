package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.1.0", 0},
		{"v0.1.0", "0.1.0", 0},
		{"0.1.0", "0.1.0-rc1", 1},  // 正式版 > 预发布
		{"0.1.0-rc1", "0.1.0", -1}, // 预发布 < 正式版
		{"0.1.0-rc2", "0.1.0-rc1", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.1.1", "0.1.0", 1},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestExpectedSHA256(t *testing.T) {
	sums := []byte("abc123  mgate-cloud_0.1.0_linux_amd64.tar.gz\ndef456 *other.zip\n")
	if h, ok := expectedSHA256(sums, "mgate-cloud_0.1.0_linux_amd64.tar.gz"); !ok || h != "abc123" {
		t.Errorf("应解析到 abc123，实际 %q ok=%t", h, ok)
	}
	if h, ok := expectedSHA256(sums, "other.zip"); !ok || h != "def456" {
		t.Errorf("应解析到 def456（去 * 前缀），实际 %q ok=%t", h, ok)
	}
	if _, ok := expectedSHA256(sums, "missing"); ok {
		t.Error("不存在的文件应返回 false")
	}
}

func TestAssetNameFor(t *testing.T) {
	name := assetNameFor("0.1.0")
	if !strings.Contains(name, "0.1.0") || !strings.Contains(name, runtime.GOOS) || !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("资产名不含版本/OS/ARCH: %s", name)
	}
	wantExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		wantExt = ".zip"
	}
	if !strings.HasSuffix(name, wantExt) {
		t.Errorf("资产名后缀应为 %s: %s", wantExt, name)
	}
}

// TestExtractFromTarGz 验证仅提取目标二进制（忽略其它条目）。
func TestExtractFromTarGz(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "pkg.tar.gz")
	payload := []byte("BINARY-CONTENT")

	// 构造含多个条目的 tar.gz，其中包含 README.md 与 nested/mgate-cloud。
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	writeTar(t, tw, "README.md", []byte("# readme"))
	writeTar(t, tw, "nested/mgate-cloud", payload)
	tw.Close()
	gz.Close()
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写归档失败: %v", err)
	}

	dest := filepath.Join(dir, "out-bin")
	if err := extractBinary(archive, dest, "mgate-cloud"); err != nil {
		t.Fatalf("提取失败: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("读取提取结果失败: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("提取内容不符: %q", got)
	}
}

func writeTar(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("写 tar 头失败: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("写 tar 体失败: %v", err)
	}
}

// TestFileSHA256RoundTrip 验证摘要计算与校验一致。
func TestFileSHA256RoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.bin")
	os.WriteFile(p, []byte("hello"), 0o644)
	sum, err := fileSHA256(p)
	if err != nil {
		t.Fatalf("计算失败: %v", err)
	}
	// "hello" 的 SHA-256。
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if sum != want {
		t.Errorf("摘要不符: %s", sum)
	}
	if err := verifySHA256(p, want); err != nil {
		t.Errorf("校验应通过: %v", err)
	}
	if err := verifySHA256(p, "deadbeef"); err == nil {
		t.Error("错误摘要应校验失败")
	}
}
