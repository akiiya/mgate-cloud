// Package update 实现保守的"检查更新 / 自更新"能力。
//
// 严格安全边界：自更新【只】下载 GitHub Release 压缩包、校验 SHA256、解压并替换
// mgate-cloud 二进制本身。它【绝不】执行下载包内的任何脚本、【不】使用 os/exec /
// exec.Command / bash / sh、【不】引入任何远程 shell 能力。
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Asset 是一个 release 资产。
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// Release 是 GitHub Release 的精简表示。
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []Asset   `json:"assets"`
}

// httpGet 发起带 UA 的 GET（GitHub API 要求 User-Agent）。
func httpGet(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mgate-cloud-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	return client.Do(req)
}

// latestRelease 按通道获取目标 release。
//
//	stable → /releases/latest（GitHub 自动排除 prerelease）
//	rc     → /releases 列表中最新的一个（含 prerelease）
func latestRelease(ctx context.Context, client *http.Client, repo, channel string) (Release, error) {
	if channel == "rc" {
		return latestFromList(ctx, client, repo)
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	resp, err := httpGet(ctx, client, url)
	if err != nil {
		return Release{}, fmt.Errorf("update: 请求 latest release 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("update: GitHub 返回状态 %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("update: 解析 release 失败: %w", err)
	}
	return rel, nil
}

// latestFromList 取 releases 列表的第一个（最新，含 prerelease）。
func latestFromList(ctx context.Context, client *http.Client, repo string) (Release, error) {
	url := "https://api.github.com/repos/" + repo + "/releases?per_page=10"
	resp, err := httpGet(ctx, client, url)
	if err != nil {
		return Release{}, fmt.Errorf("update: 请求 releases 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("update: GitHub 返回状态 %d", resp.StatusCode)
	}
	var list []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return Release{}, fmt.Errorf("update: 解析 releases 失败: %w", err)
	}
	if len(list) == 0 {
		return Release{}, fmt.Errorf("update: 仓库无 release")
	}
	return list[0], nil
}

// normalizeVersion 去掉前导 v 并裁剪空白。
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// compareSemver 比较两个版本：a>b 返回 1，a<b 返回 -1，相等 0。
//
// 支持 major.minor.patch 与可选 -prerelease；无 prerelease 视为高于有 prerelease。
func compareSemver(a, b string) int {
	a, b = normalizeVersion(a), normalizeVersion(b)
	aCore, aPre := splitPre(a)
	bCore, bPre := splitPre(b)

	an, bn := parseNums(aCore), parseNums(bCore)
	for i := 0; i < 3; i++ {
		if an[i] != bn[i] {
			if an[i] > bn[i] {
				return 1
			}
			return -1
		}
	}
	// 核心版本相等：无 prerelease > 有 prerelease。
	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "":
		return 1
	case bPre == "":
		return -1
	default:
		return strings.Compare(aPre, bPre)
	}
}

func splitPre(v string) (core, pre string) {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func parseNums(core string) [3]int {
	var out [3]int
	parts := strings.SplitN(core, ".", 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		out[i], _ = strconv.Atoi(strings.TrimSpace(parts[i]))
	}
	return out
}
