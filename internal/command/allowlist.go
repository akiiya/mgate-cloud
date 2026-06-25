package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// 参数字段长度上限：足够覆盖 SSID / 节点名等，又能挡住异常超长输入。
const maxParamStringLen = 256

// dangerousKeys 是绝不允许出现在任何命令参数中的字段名。
//
// 这是纵深防御：即便某个 action 的结构体意外放开，这层也会拦下试图夹带可执行内容的字段。
var dangerousKeys = map[string]bool{
	"shell": true, "cmd": true, "command": true, "script": true,
	"args": true, "argv": true, "extra_args": true, "raw": true,
	"raw_args": true, "exec": true, "bash": true, "sh": true,
	"password": true, "passwd": true, // 口令不属于 Phase 4 范围
}

// paramValidator 校验并规范化某个 action 的参数，返回规范化后的 JSON。
type paramValidator func(raw []byte) (json.RawMessage, error)

// allowedActions 是固定的白名单 action 及其参数规则。集中定义，禁止在别处散落字符串判断。
var allowedActions = map[string]paramValidator{
	"ap.start":       noParams,
	"ap.stop":        noParams,
	"ap.status":      noParams,
	"wlan.scan":      noParams,
	"wlan.list":      noParams,
	"tproxy.enable":  noParams,
	"tproxy.disable": noParams,
	"gateway.start":  noParams,
	"gateway.stop":   noParams,
	"gateway.status": noParams,
	"doctor.full":    noParams,
	"wlan.connect":   validateWlanConnect,
	"tproxy.use":     validateTproxyUse,
}

// IsAllowed 报告 action 是否在白名单内。
func IsAllowed(action string) bool {
	_, ok := allowedActions[action]
	return ok
}

// AllowedActions 返回所有白名单 action（排序后），供文档/前端参考。
func AllowedActions() []string {
	out := make([]string, 0, len(allowedActions))
	for a := range allowedActions {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// ValidateParams 校验 action 与其参数；返回规范化后的参数 JSON。
//
// 非白名单 action、未知字段、危险字段、非法取值一律拒绝。
func ValidateParams(action string, raw []byte) (json.RawMessage, error) {
	validator, ok := allowedActions[action]
	if !ok {
		return nil, fmt.Errorf("command: 非白名单 action: %q", action)
	}
	if err := rejectDangerousKeys(raw); err != nil {
		return nil, err
	}
	return validator(raw)
}

// rejectDangerousKeys 扫描参数对象的顶层键，命中危险名即拒绝。
func rejectDangerousKeys(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		// 非对象（如数组/标量）一律拒绝：参数必须是对象。
		return fmt.Errorf("command: 参数必须是 JSON 对象")
	}
	for k := range m {
		if dangerousKeys[strings.ToLower(strings.TrimSpace(k))] {
			return fmt.Errorf("command: 禁止的参数字段: %q", k)
		}
	}
	return nil
}

// noParams 仅接受空对象 {}（或空/缺省），其余一律拒绝。
func noParams(raw []byte) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage("{}"), nil
	}
	// 解码到无字段结构体并禁止未知字段：任何键都会被拒绝。
	var empty struct{}
	if err := strictUnmarshal(raw, &empty); err != nil {
		return nil, fmt.Errorf("command: 该操作不接受任何参数")
	}
	return json.RawMessage("{}"), nil
}

// wlanConnectParams 是 wlan.connect 的允许参数（无 password 字段）。
type wlanConnectParams struct {
	SSID      string `json:"ssid,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`
}

func validateWlanConnect(raw []byte) (json.RawMessage, error) {
	var p wlanConnectParams
	if err := strictUnmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("command: wlan.connect 参数无效")
	}
	if p.SSID == "" && p.ProfileID == "" {
		return nil, fmt.Errorf("command: wlan.connect 需提供 ssid 或 profile_id")
	}
	if err := checkText("ssid", p.SSID); err != nil {
		return nil, err
	}
	if err := checkText("profile_id", p.ProfileID); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// tproxyUseParams 是 tproxy.use 的允许参数。
type tproxyUseParams struct {
	Node string `json:"node"`
}

func validateTproxyUse(raw []byte) (json.RawMessage, error) {
	var p tproxyUseParams
	if err := strictUnmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("command: tproxy.use 参数无效")
	}
	if strings.TrimSpace(p.Node) == "" {
		return nil, fmt.Errorf("command: tproxy.use 需提供 node")
	}
	if err := checkText("node", p.Node); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// strictUnmarshal 解码 JSON 并禁止未知字段（未知字段即报错）。
func strictUnmarshal(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	// 确保没有尾随多余内容。
	if dec.More() {
		return fmt.Errorf("command: 参数包含多余内容")
	}
	return nil
}

// checkText 校验字符串参数：长度受限、不含控制字符。
func checkText(field, s string) error {
	if len(s) > maxParamStringLen {
		return fmt.Errorf("command: 参数 %s 过长", field)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("command: 参数 %s 含非法控制字符", field)
		}
	}
	return nil
}
