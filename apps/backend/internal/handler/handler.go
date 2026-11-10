// Package handler 是 HTTP 适配层。
//
// 只做三件事：解析参数、调用 service、写响应。
// 任何业务判断都不应出现在这里——它决定的是「怎么把 HTTP 翻译成一次服务调用」，
// 而不是「这件事该不该做」。
package handler

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/middleware"
)

// parseID 解析路径上的 :id 参数。
func parseID(c *gin.Context) (uint, bool) {
	return parseUintParam(c, "id", "资源")
}

// parseUintParam 解析路径上的指定参数名，供子资源用（如 :itemId）。
//
// 子资源路径上同时有 :id 与 :itemId，少一个就会把子资源 id 当成主资源 id 用，
// 而那种错误从响应里看不出来。
func parseUintParam(c *gin.Context, name, label string) (uint, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, errs.BadRequest("无效的"+label+" ID"))
		return 0, false
	}
	return uint(id), true
}

// bindJSON 绑定并校验请求体，失败时已写出响应。
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		// 请求体超过 BodyLimit 时，读错误会带着「解析失败」的外观。
		// 翻成 413：400 的意思是「你的请求写错了」，实际是「太大了」，
		// 两种提示对应的处理动作完全不同（改内容 vs 拆批次）。
		if middleware.HandleBodyTooLarge(c, err) {
			return false
		}
		response.Fail(c, errs.BadRequest(validationMessage(err)))
		return false
	}
	return true
}

// bindQuery 绑定并校验查询参数，失败时已写出响应。
func bindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		response.Fail(c, errs.BadRequest(validationMessage(err)))
		return false
	}
	return true
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
