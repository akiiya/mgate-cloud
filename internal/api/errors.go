package api

import "net/http"

// Error 是对外暴露的稳定错误结构。
//
// code 是机器可读、稳定不变的标识（前端据此判断分支）；
// message 面向人类、可中文；二者解耦，便于前端做本地化或自定义提示。
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// httpStatus 不参与 JSON 序列化，仅供 handler 决定 HTTP 状态码。
	httpStatus int `json:"-"`
}

// Error 实现 error 接口，便于在调用链中作为普通 error 传递。
func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Status 返回该错误对应的 HTTP 状态码。
func (e *Error) Status() int { return e.httpStatus }

// 预定义错误集合。
//
// 安全要点：登录失败统一返回 ErrInvalidCredentials，绝不区分"用户名不存在"
// 与"密码错误"，避免账户枚举攻击。内部错误一律以 ErrInternal 对外，不泄露细节。
var (
	ErrInvalidCredentials = &Error{Code: "invalid_credentials", Message: "用户名或密码错误", httpStatus: http.StatusUnauthorized}
	ErrUnauthorized       = &Error{Code: "unauthorized", Message: "未登录或会话已失效", httpStatus: http.StatusUnauthorized}
	ErrCSRF               = &Error{Code: "csrf_failed", Message: "CSRF 校验失败", httpStatus: http.StatusForbidden}
	ErrBadRequest         = &Error{Code: "bad_request", Message: "请求参数无效", httpStatus: http.StatusBadRequest}
	ErrMethodNotAllowed   = &Error{Code: "method_not_allowed", Message: "请求方法不被允许", httpStatus: http.StatusMethodNotAllowed}
	ErrNotFound           = &Error{Code: "not_found", Message: "资源不存在", httpStatus: http.StatusNotFound}
	ErrInternal           = &Error{Code: "internal_error", Message: "服务器内部错误", httpStatus: http.StatusInternalServerError}
	ErrNotReady           = &Error{Code: "not_ready", Message: "服务未就绪", httpStatus: http.StatusServiceUnavailable}

	// --- Phase 2：设备身份相关 ---

	// ErrDeviceNotFound 设备不存在。
	ErrDeviceNotFound = &Error{Code: "device_not_found", Message: "设备不存在", httpStatus: http.StatusNotFound}
	// ErrPairingNotAllowed 当前设备状态不允许生成设备码（仅 pending 可生成）。
	ErrPairingNotAllowed = &Error{Code: "pairing_not_allowed", Message: "当前设备状态不允许生成设备码", httpStatus: http.StatusConflict}

	// 以下错误用于 agent enroll。
	// 安全取舍：结构性失败（格式错误、签名不符、查无此码）统一映射为
	// ErrInvalidPairingCode，避免攻击者据响应差异区分原因；仅"过期/已用/禁用"
	// 这类状态性结果给出独立 code，便于 agent 给用户准确提示。
	ErrInvalidPairingCode    = &Error{Code: "invalid_pairing_code", Message: "设备码无效或已过期", httpStatus: http.StatusBadRequest}
	ErrExpiredPairingCode    = &Error{Code: "expired_pairing_code", Message: "设备码已过期", httpStatus: http.StatusBadRequest}
	ErrUsedPairingCode       = &Error{Code: "used_pairing_code", Message: "设备码已被使用", httpStatus: http.StatusConflict}
	ErrDeviceDisabled        = &Error{Code: "device_disabled", Message: "设备已被禁用", httpStatus: http.StatusForbidden}
	ErrDeviceAlreadyEnrolled = &Error{Code: "device_already_enrolled", Message: "设备已完成绑定", httpStatus: http.StatusConflict}
	ErrInvalidRequest        = &Error{Code: "invalid_request", Message: "请求体无效", httpStatus: http.StatusBadRequest}

	// --- Phase 4：命令队列相关 ---

	// ErrDeviceOffline 设备当前不在线，Phase 4 仅允许对在线设备下发命令。
	ErrDeviceOffline = &Error{Code: "device_offline", Message: "设备不在线，无法下发命令", httpStatus: http.StatusConflict}
	// ErrDeviceNotReady 设备状态不允许下发命令（pending / disabled）。
	ErrDeviceNotReady = &Error{Code: "device_not_ready", Message: "设备状态不允许下发命令", httpStatus: http.StatusConflict}
	// ErrInvalidAction action 不在白名单内。
	ErrInvalidAction = &Error{Code: "invalid_action", Message: "不支持的操作", httpStatus: http.StatusBadRequest}
	// ErrInvalidParams 命令参数不合法。
	ErrInvalidParams = &Error{Code: "invalid_params", Message: "命令参数不合法", httpStatus: http.StatusBadRequest}
	// ErrTimeoutTooLarge 指定的超时超过允许上限。
	ErrTimeoutTooLarge = &Error{Code: "timeout_too_large", Message: "超时设置超过允许上限", httpStatus: http.StatusBadRequest}
	// ErrCommandNotFound 命令不存在。
	ErrCommandNotFound = &Error{Code: "command_not_found", Message: "命令不存在", httpStatus: http.StatusNotFound}
	// ErrCommandNotCancelable 命令已处于终态，无法取消。
	ErrCommandNotCancelable = &Error{Code: "command_not_cancelable", Message: "命令已结束，无法取消", httpStatus: http.StatusConflict}
)

// NewError 构造自定义错误，供需要特定 code/message 的场景使用。
func NewError(code, message string, httpStatus int) *Error {
	return &Error{Code: code, Message: message, httpStatus: httpStatus}
}
