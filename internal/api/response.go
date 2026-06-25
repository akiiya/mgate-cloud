// Package api 定义稳定的 HTTP JSON 响应契约与错误类型。
//
// 全站统一使用 {ok, data} / {ok, error} 两种信封，前端无需为每个接口猜测结构。
package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// envelope 是所有响应的统一外层结构。
// 成功时 Data 有值、Error 为 nil；失败时相反。omitempty 保证另一字段不出现。
type envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// WriteSuccess 输出成功响应：{"ok":true,"data":...}。
//
// data 为 nil 时仍返回 {"ok":true}，对"无返回体"的操作（如登出）友好。
func WriteSuccess(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, envelope{OK: true, Data: data})
}

// WriteError 输出失败响应：{"ok":false,"error":{...}}，HTTP 状态码取自错误自身。
//
// 接受 error 接口：若是已知的 *api.Error 则原样输出；否则视为内部错误，
// 仅对外暴露通用 internal_error，绝不把原始错误细节泄露给客户端。
func WriteError(w http.ResponseWriter, err error) {
	apiErr, ok := err.(*Error)
	if !ok {
		// 未知错误：记录到服务端日志便于排查，但对外只给通用信息。
		log.Printf("api: 未分类的内部错误: %v", err)
		apiErr = ErrInternal
	}
	writeJSON(w, apiErr.Status(), envelope{OK: false, Error: apiErr})
}

// writeJSON 是底层序列化与写出，统一设置 Content-Type 与编码。
func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// 此时响应头已写出，无法再改状态码，只能记录日志。
		log.Printf("api: 响应序列化失败: %v", err)
	}
}
