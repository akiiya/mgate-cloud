package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// sumsAssetName 是 release 中校验和文件名。
const sumsAssetName = "SHA256SUMS"

// Service 编排检查更新与自更新。
type Service struct {
	client  *http.Client
	repo    string
	channel string
	current string
	enabled bool
}

// NewService 构造更新服务。
func NewService(repo, channel, current string, enabled bool) *Service {
	return &Service{
		client:  &http.Client{Timeout: 60 * time.Second},
		repo:    repo,
		channel: channel,
		current: current,
		enabled: enabled,
	}
}

// Enabled 报告更新功能是否启用。
func (s *Service) Enabled() bool { return s.enabled }

// CheckResult 是检查更新结果。
type CheckResult struct {
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version"`
	HasUpdate      bool      `json:"has_update"`
	PublishedAt    time.Time `json:"published_at"`
	HTMLURL        string    `json:"html_url"`
	AssetName      string    `json:"asset_name"`
	AssetAvailable bool      `json:"asset_available"`
	Channel        string    `json:"channel"`
}

// Check 查询最新 release 并与当前版本比较。
func (s *Service) Check(ctx context.Context) (CheckResult, error) {
	rel, err := latestRelease(ctx, s.client, s.repo, s.channel)
	if err != nil {
		return CheckResult{}, err
	}
	latest := normalizeVersion(rel.TagName)
	assetName := assetNameFor(latest)
	_, assetOK := findAsset(rel, assetName)

	return CheckResult{
		CurrentVersion: normalizeVersion(s.current),
		LatestVersion:  latest,
		HasUpdate:      compareSemver(latest, s.current) > 0,
		PublishedAt:    rel.PublishedAt,
		HTMLURL:        rel.HTMLURL,
		AssetName:      assetName,
		AssetAvailable: assetOK,
		Channel:        s.channel,
	}, nil
}

// ApplyResult 是自更新结果。
type ApplyResult struct {
	Version      string `json:"version"`
	Replaced     bool   `json:"replaced"`
	NeedsManual  bool   `json:"needs_manual"`
	NeedsRestart bool   `json:"needs_restart"`
	StagedPath   string `json:"staged_path,omitempty"`
	BackupPath   string `json:"backup_path,omitempty"`
	Message      string `json:"message"`
}

// Apply 执行自更新：下载 → 校验 SHA256 → 解压出二进制 → 备份 → 原子替换。
//
// 仅替换 mgate-cloud 二进制本身；绝不执行包内任何脚本，绝不调用外部命令。
func (s *Service) Apply(ctx context.Context) (ApplyResult, error) {
	rel, err := latestRelease(ctx, s.client, s.repo, s.channel)
	if err != nil {
		return ApplyResult{}, err
	}
	latest := normalizeVersion(rel.TagName)

	if compareSemver(latest, s.current) <= 0 {
		return ApplyResult{Version: latest, Message: "已是最新版本，无需更新"}, nil
	}

	assetName := assetNameFor(latest)
	archiveAsset, ok := findAsset(rel, assetName)
	if !ok {
		return ApplyResult{}, fmt.Errorf("update: 该平台无对应资产 %s", assetName)
	}
	sumsAsset, ok := findAsset(rel, sumsAssetName)
	if !ok {
		return ApplyResult{}, fmt.Errorf("update: release 缺少 %s", sumsAssetName)
	}

	staging, err := os.MkdirTemp("", "mgate-update-*")
	if err != nil {
		return ApplyResult{}, fmt.Errorf("update: 创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(staging)

	// 下载压缩包并校验 SHA256。
	archivePath := filepath.Join(staging, assetName)
	if err := downloadToFile(ctx, s.client, archiveAsset.DownloadURL, archivePath); err != nil {
		return ApplyResult{}, err
	}
	sums, err := downloadBytes(ctx, s.client, sumsAsset.DownloadURL)
	if err != nil {
		return ApplyResult{}, err
	}
	expected, ok := expectedSHA256(sums, assetName)
	if !ok {
		return ApplyResult{}, fmt.Errorf("update: SHA256SUMS 中无 %s", assetName)
	}
	if err := verifySHA256(archivePath, expected); err != nil {
		return ApplyResult{}, err // 校验失败必须中止
	}

	// 仅从压缩包中解出 mgate-cloud 二进制（忽略其余文件，绝不执行任何脚本）。
	binName := binaryName()
	stagedBin := filepath.Join(staging, "new-"+binName)
	if err := extractBinary(archivePath, stagedBin, binName); err != nil {
		return ApplyResult{}, err
	}

	exePath, err := os.Executable()
	if err != nil {
		return ApplyResult{}, fmt.Errorf("update: 无法定位当前二进制: %w", err)
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	// 备份当前二进制。
	backupPath := exePath + ".bak"
	if err := copyFile(exePath, backupPath, 0o755); err != nil {
		return ApplyResult{}, fmt.Errorf("update: 备份当前二进制失败: %w", err)
	}

	// Windows 无法替换正在运行的 exe：放到 .new 并提示手动替换。
	if runtime.GOOS == "windows" {
		newPath := exePath + ".new"
		if err := copyFile(stagedBin, newPath, 0o755); err != nil {
			return ApplyResult{}, fmt.Errorf("update: 写入新二进制失败: %w", err)
		}
		return ApplyResult{
			Version: latest, Replaced: false, NeedsManual: true, NeedsRestart: true,
			StagedPath: newPath, BackupPath: backupPath,
			Message: "Windows 无法替换运行中的程序：新版本已下载到 .new，请停止服务后用其覆盖当前 mgate-cloud.exe（旧版本已备份为 .bak）",
		}, nil
	}

	// Unix：写到同目录临时文件，再原子 rename 覆盖（同一文件系统）。
	tmpPath := filepath.Join(filepath.Dir(exePath), "."+binName+".new")
	if err := copyFile(stagedBin, tmpPath, 0o755); err != nil {
		return ApplyResult{}, fmt.Errorf("update: 写入新二进制失败: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Remove(tmpPath)
		return ApplyResult{}, fmt.Errorf("update: 替换二进制失败（旧版本已备份为 %s）: %w", backupPath, err)
	}

	return ApplyResult{
		Version: latest, Replaced: true, NeedsRestart: true, BackupPath: backupPath,
		Message: "更新已安装，请重启服务以生效（systemd 下可直接重启进程，旧版本已备份为 .bak）",
	}, nil
}

// assetNameFor 按当前 OS/ARCH 生成资产名，与 release 打包命名一致。
func assetNameFor(version string) string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("mgate-cloud_%s_%s_%s%s", version, runtime.GOOS, runtime.GOARCH, ext)
}

// binaryName 是当前平台的二进制文件名。
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "mgate-cloud.exe"
	}
	return "mgate-cloud"
}

// findAsset 在 release 中按名查找资产。
func findAsset(rel Release, name string) (Asset, bool) {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// extractBinary 仅从压缩包中提取名为 binName 的二进制到 dest（忽略其它条目）。
func extractBinary(archivePath, dest, binName string) error {
	if filepath.Ext(archivePath) == ".zip" {
		return extractFromZip(archivePath, dest, binName)
	}
	return extractFromTarGz(archivePath, dest, binName)
}

func extractFromTarGz(archivePath, dest, binName string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("update: gzip 解码失败: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("update: 读取 tar 失败: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == binName {
			return writeStream(io.LimitReader(tr, maxDownloadBytes), dest, 0o755)
		}
	}
	return fmt.Errorf("update: 压缩包中未找到 %s", binName)
}

func extractFromZip(archivePath, dest, binName string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("update: 打开 zip 失败: %w", err)
	}
	defer zr.Close()
	for _, file := range zr.File {
		if filepath.Base(file.Name) == binName {
			rc, err := file.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeStream(io.LimitReader(rc, maxDownloadBytes), dest, 0o755)
		}
	}
	return fmt.Errorf("update: 压缩包中未找到 %s", binName)
}

// writeStream 将 r 写入 dest（覆盖），并设置权限。
func writeStream(r io.Reader, dest string, perm os.FileMode) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return out.Chmod(perm)
}

// copyFile 复制文件并设置权限。
func copyFile(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeStream(in, dest, perm)
}
