// Package handler 是 HTTP 适配层。
//
// 只做三件事：解析参数、调用 service、返回统一响应体。
// 任何业务判断都不应出现在这里——它决定的是「怎么把 HTTP 翻译成一次服务调用」，
// 而不是「这件事该不该做」。
//
// 本包依赖 Fuego 的上下文类型，但只用到三样东西：取请求、取响应、取路径参数。
// 解析与校验一律走本文件里的 bindJSON / bindQuery，而不是框架自带的绑定：
// 那套绑定会把错误折叠成它自己的英文提示，与既有文案对不上。
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/schema"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
)

// validate 与改造前用的是同一套校验语义：go-playground/validator，
// 但标签名必须显式改成 binding —— 它的默认标签名是 validate，
// 不改的话 dto 上的 binding 约束会被静默忽略，校验看上去「通过了」。
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()
	v.SetTagName("binding")
	return v
}

// queryDecoder 按 dto 上的 form 标签解析查询参数（嵌入的 PageQuery 一并处理）。
var queryDecoder = newQueryDecoder()

func newQueryDecoder() *schema.Decoder {
	d := schema.NewDecoder()
	d.SetAliasTag("form")
	// 未知参数不报错：前端多带一个时间戳之类的参数不该变成 400。
	d.IgnoreUnknownKeys(true)
	return d
}

// bindJSON 把请求体解成 dst 并校验。
//
// 返回的可能是 *errs.Error（交给框架输出），也可能是 response.ErrResponseWritten
// （413 已经写出，不能再写第二次）。
func bindJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		// 请求体超过 BodyLimit 时，读错误会带着「解析失败」的外观。
		if response.HandleTooLarge(w, err) {
			return response.ErrResponseWritten
		}
		return errs.BadRequest(validationMessage(err))
	}
	if err := validate.Struct(dst); err != nil {
		return errs.BadRequest(validationMessage(err))
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
