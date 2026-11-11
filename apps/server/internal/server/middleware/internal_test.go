package middleware

// 本文件测两个没有 HTTP 语义就无法验证的小函数。
//
// 它们都在安全边界上：validRequestID 决定「上游传来的值能不能进日志与响应头」，
// clientIP 决定限流的桶怎么分。前者放过一个换行符就能伪造日志行，
// 后者分错桶等于限流白配——两者都不会报错，只会在某天出问题时才被发现。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidRequestID(t *testing.T) {
	valid := []string{
		"abc",
		"0f3a-11b2_9.x",
		strings.Repeat("a", 64), // 正好上限
	}
	for _, id := range valid {
		if !validRequestID(id) {
			t.Errorf("validRequestID(%q) = false，期望放行", id)
		}
	}

	invalid := map[string]string{
		"空串":      "",
		"超长":      strings.Repeat("a", 65),
		"含空格":     "a b",
		"含斜杠":     "a/b",
		"含换行":     "a\nb",
		"非 ASCII": "请求-1",
		"含引号":     `a"b`,
		"含分号（注入）": "a;rm -rf /",
	}
	for name, id := range invalid {
		if validRequestID(id) {
			t.Errorf("%s：validRequestID(%q) = true，期望拒绝", name, id)
		}
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		header     map[string]string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For 取第一跳",
			header:     map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"},
			remoteAddr: "10.0.0.1:5000",
			want:       "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For 单值",
			header:     map[string]string{"X-Forwarded-For": " 1.2.3.4 "},
			remoteAddr: "10.0.0.1:5000",
			want:       "1.2.3.4",
		},
		{
			name:       "退回 X-Real-IP",
			header:     map[string]string{"X-Real-IP": "2.3.4.5"},
			remoteAddr: "10.0.0.1:5000",
			want:       "2.3.4.5",
		},
		{
			name:       "最后退回连接地址并去掉端口",
			remoteAddr: "9.9.9.9:1234",
			want:       "9.9.9.9",
		},
		{
			name:       "连接地址没有端口时原样返回",
			remoteAddr: "9.9.9.9",
			want:       "9.9.9.9",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
			for k, v := range c.header {
				r.Header.Set(k, v)
			}
			r.RemoteAddr = c.remoteAddr

			if got := clientIP(r); got != c.want {
				t.Errorf("clientIP = %q，期望 %q", got, c.want)
			}
		})
	}
}
