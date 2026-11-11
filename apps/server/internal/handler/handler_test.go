package handler

// 本文件守住 handler 层三个「翻译器」：校验错误 → 中文文案、路径参数 → ID、
// Content-Type → 是否 JSON。
//
// 三者都只在边界上出错：文案错了用户看不懂，ID 解析错了会查错记录，
// Content-Type 放行了会走到框架的类型断言里 panic。端到端用例只覆盖到其中
// 一两条分支，其余靠这里逐个点名。

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestValidationMessage 覆盖每个被翻译的标签，外加「不是校验错误」的兜底。
func TestValidationMessage(t *testing.T) {
	cases := []struct {
		name string
		dst  any
		want string
	}{
		{
			name: "required",
			dst: &struct {
				Title string `binding:"required"`
			}{},
			want: "参数 Title 不能为空",
		},
		{
			name: "email",
			dst: &struct {
				Mail string `binding:"email"`
			}{Mail: "not-an-email"},
			want: "参数 Mail 不是合法的邮箱",
		},
		{
			name: "max",
			dst: &struct {
				Title string `binding:"max=5"`
			}{Title: "123456"},
			want: "参数 Title 长度不能超过 5",
		},
		{
			name: "min",
			dst: &struct {
				Title string `binding:"min=3"`
			}{Title: "ab"},
			want: "参数 Title 长度不能少于 3",
		},
		{
			name: "gte",
			dst: &struct {
				Age int `binding:"gte=18"`
			}{Age: 1},
			want: "参数 Age 不能小于 18",
		},
		{
			// 没翻译的标签也要给出可定位的提示，而不是英文原句。
			name: "未翻译的标签走兜底",
			dst: &struct {
				Kind string `binding:"oneof=a b"`
			}{Kind: "c"},
			want: "参数 Kind 校验失败(oneof)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validate.Struct(c.dst)
			if err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
			if got := validationMessage(err); got != c.want {
				t.Errorf("validationMessage = %q，期望 %q", got, c.want)
			}
		})
	}

	if got := validationMessage(errors.New("不是校验错误")); got != "请求参数格式不正确" {
		t.Errorf("非校验错误的文案 = %q，期望兜底文案", got)
	}
}

// TestParseID 覆盖 0、非数字、负数与溢出：它们都必须是 400，而不是「查不到」的 404。
func TestParseID(t *testing.T) {
	valid := map[string]uint{"1": 1, "42": 42}
	for raw, want := range valid {
		got, err := parseID(raw)
		if err != nil || got != want {
			t.Errorf("parseID(%q) = %d, %v；期望 %d, nil", raw, got, err, want)
		}
	}

	invalid := []string{"", "0", "abc", "-1", "1.5", "99999999999999999999"}
	for _, raw := range invalid {
		if _, err := parseID(raw); err == nil {
			t.Errorf("parseID(%q) 通过了，期望报参数错误", raw)
		}
	}
}

// TestEnsureJSONBody 只放行 JSON，其余在进业务之前拒掉。
//
// Fuego 的 text/plain 分支会对结构体做类型断言（直接 panic），所以这一层不是
// 「更严格一点」，而是替代框架那条会崩的路径。
func TestEnsureJSONBody(t *testing.T) {
	cases := []struct {
		contentType string
		wantErr     bool
	}{
		{"", false}, // 不写 Content-Type 时按 JSON 处理，与改造前一致
		{"application/json", false},
		{"application/json; charset=utf-8", false},
		{"text/plain", true},
		{"application/x-www-form-urlencoded", true},
		{"not a media type", true},
	}

	for _, c := range cases {
		r, err := http.NewRequest(http.MethodPost, "/api/v1/notes", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("构造请求: %v", err)
		}
		if c.contentType != "" {
			r.Header.Set("Content-Type", c.contentType)
		}

		err = ensureJSONBody(r)
		if c.wantErr && err == nil {
			t.Errorf("Content-Type=%q 被放行了，期望拒绝", c.contentType)
		}
		if !c.wantErr && err != nil {
			t.Errorf("Content-Type=%q 被拒绝：%v", c.contentType, err)
		}
	}
}
