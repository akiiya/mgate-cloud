package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// newTestFS 构造一个内存文件系统模拟 web/dist。
func newTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>spa</title>")},
		"assets/app.js": {Data: []byte("console.log('hi')")},
	}
}

// TestServeExistingAsset 验证真实静态文件被正确返回。
func TestServeExistingAsset(t *testing.T) {
	h := NewSPAHandler(newTestFS())

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	if body := rec.Body.String(); body != "console.log('hi')" {
		t.Errorf("资源内容不符: %q", body)
	}
}

// TestServeRootReturnsIndex 验证根路径返回 index.html。
func TestServeRootReturnsIndex(t *testing.T) {
	h := NewSPAHandler(newTestFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Error("应设置 Content-Type")
	}
}

// TestUnknownPathFallsBackToIndex 验证未知路径回退到 index.html（SPA 刷新不 404）。
func TestUnknownPathFallsBackToIndex(t *testing.T) {
	h := NewSPAHandler(newTestFS())

	req := httptest.NewRequest(http.MethodGet, "/some/deep/link", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("未知路径应回退 index 返回 200，实际 %d", rec.Code)
	}
	if body := rec.Body.String(); body == "" {
		t.Error("回退响应体不应为空")
	}
}
