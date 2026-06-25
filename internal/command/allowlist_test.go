package command

import "testing"

// TestAllAllowedActionsAccepted 验证所有白名单 action 被识别，无参数 action 接受 {}。
func TestAllAllowedActionsAccepted(t *testing.T) {
	noParam := []string{
		"ap.start", "ap.stop", "ap.status", "wlan.scan", "wlan.list",
		"tproxy.enable", "tproxy.disable", "gateway.start", "gateway.stop",
		"gateway.status", "doctor.full",
	}
	for _, a := range noParam {
		if !IsAllowed(a) {
			t.Errorf("%s 应在白名单", a)
		}
		if _, err := ValidateParams(a, []byte("{}")); err != nil {
			t.Errorf("%s 接受空参数失败: %v", a, err)
		}
		if _, err := ValidateParams(a, nil); err != nil {
			t.Errorf("%s 接受 nil 参数失败: %v", a, err)
		}
	}
}

// TestInvalidActionRejected 验证非白名单 action 被拒绝。
func TestInvalidActionRejected(t *testing.T) {
	if IsAllowed("exec.raw") {
		t.Error("exec.raw 不应在白名单")
	}
	if _, err := ValidateParams("exec.raw", []byte("{}")); err == nil {
		t.Error("非白名单 action 应被拒绝")
	}
}

// TestNoParamActionRejectsUnknownField 验证无参数 action 带字段被拒绝。
func TestNoParamActionRejectsUnknownField(t *testing.T) {
	if _, err := ValidateParams("ap.status", []byte(`{"foo":"bar"}`)); err == nil {
		t.Error("无参数 action 带未知字段应被拒绝")
	}
}

// TestWlanConnectParams 验证 wlan.connect 参数规则。
func TestWlanConnectParams(t *testing.T) {
	if _, err := ValidateParams("wlan.connect", []byte(`{"ssid":"HomeWiFi"}`)); err != nil {
		t.Errorf("合法 ssid 应通过: %v", err)
	}
	if _, err := ValidateParams("wlan.connect", []byte(`{"profile_id":"wifi_1"}`)); err != nil {
		t.Errorf("合法 profile_id 应通过: %v", err)
	}
	if _, err := ValidateParams("wlan.connect", []byte(`{}`)); err == nil {
		t.Error("空 wlan.connect 应被拒绝")
	}
	if _, err := ValidateParams("wlan.connect", []byte(`{"ssid":"x","password":"secret"}`)); err == nil {
		t.Error("wlan.connect 含 password 应被拒绝")
	}
}

// TestTproxyUseParams 验证 tproxy.use 参数规则。
func TestTproxyUseParams(t *testing.T) {
	if _, err := ValidateParams("tproxy.use", []byte(`{"node":"US"}`)); err != nil {
		t.Errorf("合法 node 应通过: %v", err)
	}
	if _, err := ValidateParams("tproxy.use", []byte(`{}`)); err == nil {
		t.Error("缺 node 应被拒绝")
	}
}

// TestDangerousFieldsRejected 验证危险字段一律被拒绝。
func TestDangerousFieldsRejected(t *testing.T) {
	dangerous := []string{
		`{"shell":"x"}`, `{"cmd":"x"}`, `{"command":"x"}`, `{"script":"x"}`,
		`{"args":["x"]}`, `{"argv":["x"]}`, `{"extra_args":"x"}`, `{"raw":"x"}`,
	}
	for _, raw := range dangerous {
		if _, err := ValidateParams("ap.status", []byte(raw)); err == nil {
			t.Errorf("危险字段应被拒绝: %s", raw)
		}
		// 即便对带参 action，危险字段也应被拒绝。
		if _, err := ValidateParams("wlan.connect", []byte(raw)); err == nil {
			t.Errorf("wlan.connect 危险字段应被拒绝: %s", raw)
		}
	}
}

// TestParamsMustBeObject 验证非对象参数被拒绝。
func TestParamsMustBeObject(t *testing.T) {
	for _, raw := range []string{`[]`, `"x"`, `123`, `true`} {
		if _, err := ValidateParams("ap.status", []byte(raw)); err == nil {
			t.Errorf("非对象参数应被拒绝: %s", raw)
		}
	}
}
