// Package securitycheck 通过静态扫描守护核心安全边界：
// 仓库源码中【不得】出现任意命令执行 / 远程 shell 能力。
//
// 该测试是 CI 的硬闸门：一旦有人误引入 os/exec、exec.Command、bash -c、sh -c，
// 测试立即失败，阻止合入。
package securitycheck

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot 由本测试文件位置推导：internal/securitycheck/ 的上两级即仓库根。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// 禁止的导入路径与代码 token。
var (
	forbiddenImports = []string{"os/exec"}
	forbiddenTokens  = []string{"exec.Command", "exec.CommandContext", "bash -c", "sh -c"}
)

// TestNoForbiddenExecCapabilities 扫描所有非测试 .go 源码，确认无任意命令执行能力。
func TestNoForbiddenExecCapabilities(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// 跳过无关目录。
			switch d.Name() {
			case ".git", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		// 只检查 .go 源码；跳过 _test.go（测试中含模式串，避免自匹配）。
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// 1) 导入检查：禁止 os/exec（无此导入则无法调用 exec.Command）。
		af, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("解析 %s 失败: %v", path, perr)
			return nil
		}
		for _, imp := range af.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenImports {
				if p == bad {
					t.Errorf("禁止的导入 %q 出现在 %s", bad, rel(root, path))
				}
			}
		}

		// 2) token 检查：剔除行注释后再匹配，避免命中"禁止事项"说明文字。
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Errorf("读取 %s 失败: %v", path, rerr)
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			code := line
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx] // 去掉行注释部分
			}
			for _, tok := range forbiddenTokens {
				if strings.Contains(code, tok) {
					t.Errorf("禁止的用法 %q 出现在 %s:%d", tok, rel(root, path), i+1)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描源码失败: %v", err)
	}
}

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}
