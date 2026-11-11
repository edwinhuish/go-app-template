package response_test

// 本文件守住「哪一档错误透出文案、哪一档折叠」这条分界。
//
// 它值得单独测：折叠规则写在 Fail 里，调用方却在 service 与中间件两处，
// 谁都可以很自然地写一句 errs.Wrap(err, CodeInternal, "查询账号失败")。
// 这条用例就是那句写法的守门人——文案折了，码不能折。

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
)

func TestFailMapsAndFolds(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    int
		wantMessage string
	}{
		{
			name:        "业务 4xx 原样透出",
			err:         errs.BadRequest("标题不能为空"),
			wantStatus:  http.StatusBadRequest,
			wantCode:    40001,
			wantMessage: "标题不能为空",
		},
		{
			name:        "业务化的 500 折叠文案",
			err:         errs.Wrap(errors.New("sql: no such column"), errs.CodeInternal, "查询账号失败"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    50001,
			wantMessage: "服务器内部错误",
		},
		{
			name:        "非业务错误折叠成 500",
			err:         errors.New("panic: runtime error"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    50001,
			wantMessage: "服务器内部错误",
		},
		{
			name:        "超时是 504，文案保留动作提示",
			err:         errs.ErrTimeout,
			wantStatus:  http.StatusGatewayTimeout,
			wantCode:    50401,
			wantMessage: "请求处理超时，请稍后重试",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			response.Fail(w, log, response.RequestInfo{
				Method:    http.MethodGet,
				Path:      "/api/v1/notes",
				RequestID: "req-1",
			}, c.err)

			if w.Code != c.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d", w.Code, c.wantStatus)
			}

			var body response.ErrorBody
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("解析响应体 %q: %v", w.Body.String(), err)
			}
			if body.Code != c.wantCode {
				t.Errorf("code = %d，期望 %d", body.Code, c.wantCode)
			}
			if body.Message != c.wantMessage {
				t.Errorf("message = %q，期望 %q", body.Message, c.wantMessage)
			}
			if body.RequestID != "req-1" {
				t.Errorf("request_id = %q，期望原样带回", body.RequestID)
			}
		})
	}
}

// TestFailSkipsWrittenResponse 验证「响应已经写出」时不再追加一段 JSON。
func TestFailSkipsWrittenResponse(t *testing.T) {
	w := httptest.NewRecorder()
	w.WriteHeader(http.StatusRequestEntityTooLarge)

	response.Fail(w, nil, response.RequestInfo{}, response.ErrResponseWritten)

	if got := w.Body.Len(); got != 0 {
		t.Errorf("响应体长度 = %d，期望 0（已经写出的响应不该被追加内容）", got)
	}
}
