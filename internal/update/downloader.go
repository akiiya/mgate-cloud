package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// maxDownloadBytes 限制单个下载的大小，防止异常超大响应。
const maxDownloadBytes = 100 << 20 // 100 MiB

// downloadToFile 把 url 下载到 dest 文件。
func downloadToFile(ctx context.Context, client *http.Client, url, dest string) error {
	resp, err := httpGet(ctx, client, url)
	if err != nil {
		return fmt.Errorf("update: 下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update: 下载返回状态 %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("update: 创建临时文件失败: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, io.LimitReader(resp.Body, maxDownloadBytes)); err != nil {
		return fmt.Errorf("update: 写入下载文件失败: %w", err)
	}
	return nil
}

// downloadBytes 下载小文件（如 SHA256SUMS）到内存。
func downloadBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	resp, err := httpGet(ctx, client, url)
	if err != nil {
		return nil, fmt.Errorf("update: 下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: 下载返回状态 %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// fileSHA256 计算文件的 SHA-256 十六进制摘要。
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// expectedSHA256 从 SHA256SUMS 内容中查出指定文件名对应的摘要。
//
// 行格式："<hex>  <filename>" 或 "<hex> *<filename>"（二进制模式）。
func expectedSHA256(sums []byte, filename string) (string, bool) {
	for _, line := range strings.Split(string(sums), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hexSum := fields[0]
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == filename {
			return strings.ToLower(hexSum), true
		}
	}
	return "", false
}

// verifySHA256 校验文件摘要是否与期望一致。
func verifySHA256(path, expected string) error {
	got, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("update: 计算摘要失败: %w", err)
	}
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("update: SHA256 校验失败（期望 %s，实际 %s）", expected, got)
	}
	return nil
}
