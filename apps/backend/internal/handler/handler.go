// Package handler 是 HTTP 适配层。
//
// 只做三件事：解析参数、调用 service、返回统一响应体。
// 任何业务判断都不应出现在这里——它决定的是「怎么把 HTTP 翻译成一次服务调用」，
// 而不是「这件事该不该做」。
//
// 本包依赖 Fuego 的上下文类型，用到的是：取请求、取响应、取路径参数、读请求体。
// 请求体走框架的绑定（c.Body()），查询参数走 gorilla/schema —— 后者是因为框架
// 只按 OptionQuery* 生成文档、不做运行时解析。两条路都用同一个 validator 实例。
package handler

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-fuego/fuego"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/schema"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
)

// validate 是全包唯一（也是交给 Fuego 的那一个）校验器实例。
//
// 标签名必须显式改成 binding：validator 默认读 validate，不改的话 dto 上的
// binding 约束会被静默忽略，校验看上去「通过了」。两个实例各配一次标签名的做法
// 更危险——配错一个就有一半接口不校验，所以这里只留一个，见 Validator。
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()
	v.SetTagName("binding")
	return v
}

// Validator 把本包的校验器交给 Fuego 复用（在 server 装配时注入）。
//
// 必须是同一个实例：Fuego 的 c.Body() 用它校验请求体，bindQuery 用它校验查询参数，
// 文档的 schema 生成又读同一批 binding 标签——三者共用一个配置，才不会出现
// 「文档要求 1~128 字、实际只校验非空」这种漂移。
func Validator() *validator.Validate { return validate }

// queryDecoder 按 dto 上的 form 标签解析查询参数（嵌入的 PageQuery 一并处理）。
var queryDecoder = newQueryDecoder()

func newQueryDecoder() *schema.Decoder {
	d := schema.NewDecoder()
	d.SetAliasTag("form")
	// 未知参数不报错：前端多带一个时间戳之类的参数不该变成 400。
	d.IgnoreUnknownKeys(true)
	return d
}

// readBody 读请求体：解码与校验都交给框架，错误翻成本项目的错误类型。
//
// 返回的可能是 *errs.Error（交给框架输出），也可能是 response.ErrResponseWritten
// （413 已经写出，不能再写第二次）。
func readBody[T any](c fuego.ContextWithBody[T]) (T, error) {
	var zero T

	if err := ensureJSONBody(c.Request()); err != nil {
		return zero, err
	}

	body, err := c.Body()
	// Fuego 在 body() 里无条件往响应头加 Server-Timing（不受 WithoutFlowHeaders
	// 控制）。本服务的对外响应头里没有它，这里删掉——否则只有带请求体的接口
	// 会凭空多一个头，网关与客户端的白名单就得分接口写。
	c.Response().Header().Del("Server-Timing")
	if err != nil {
		// 请求体超过 BodyLimit 时，读错误会带着「解析失败」的外观。
		if response.HandleTooLarge(c.Response(), err) {
			return zero, response.ErrResponseWritten
		}
		// Fuego 的校验错误是 HTTPError{Err: validator.ValidationErrors}，且实现了
		// Unwrap；validationMessage 顺着这条链取字段与标签，文案仍是中文。
		return zero, errs.BadRequest(validationMessage(err))
	}
	return body, nil
}

// ensureJSONBody 拒绝非 JSON 的请求体。
//
// Fuego 的 body() 按 Content-Type 选解码器：text/plain 分支走的是 `any(s).(B)`
// 类型断言，对结构体请求体会直接 panic；form / xml / yaml 各有自己的解析路径。
// 本服务对外只有 JSON，在这里提前拒掉，别把框架的分支暴露给调用方。
func ensureJSONBody(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		// 不写 Content-Type 时 Fuego 默认按 JSON 解析，与改造前一致。
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil || mediaType != "application/json" {
		return errs.BadRequest("请求体必须是 application/json")
	}
	return nil
}

// bindQuery 把查询参数解成 dst 并校验。
func bindQuery(r *http.Request, dst any) error {
	if err := queryDecoder.Decode(dst, r.URL.Query()); err != nil {
		return errs.BadRequest(validationMessage(err))
	}
	if err := validate.Struct(dst); err != nil {
		return errs.BadRequest(validationMessage(err))
	}
	return nil
}

// parseID 解析路径上的 id 参数。0 与非数字一律当参数错误，而不是去查一条不存在的记录：
// 后者会返回 404，让人以为是数据没了。
//
// 位宽取 strconv.IntSize 而不是 64：在 32 位平台上 uint 装不下 2^32，
// 直接强转会把 4294967297 截成 1，一次「查不存在的记录」变成「查到了别人的记录」。
func parseID(raw string) (uint, error) {
	id, err := strconv.ParseUint(raw, 10, strconv.IntSize)
	if err != nil || id == 0 {
		return 0, errs.BadRequest("无效的资源 ID")
	}
	return uint(id), nil
}

// validationMessage 把校验错误翻译成可定位的中文提示。
//
// 直接把 validator 的英文原句抛给用户，会出现「Key: 'X.Field' Error:Field
// validation for 'Field' failed on the 'required' tag」这类没人看得懂的内容。
func validationMessage(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) && len(ve) > 0 {
		fe := ve[0]
		field := fe.Field()
		switch fe.Tag() {
		case "required":
			return fmt.Sprintf("参数 %s 不能为空", field)
		case "email":
			return fmt.Sprintf("参数 %s 不是合法的邮箱", field)
		case "max":
			return fmt.Sprintf("参数 %s 长度不能超过 %s", field, fe.Param())
		case "min":
			return fmt.Sprintf("参数 %s 长度不能少于 %s", field, fe.Param())
		case "gte":
			return fmt.Sprintf("参数 %s 不能小于 %s", field, fe.Param())
		default:
			return fmt.Sprintf("参数 %s 校验失败(%s)", field, fe.Tag())
		}
	}
	return "请求参数格式不正确"
}
