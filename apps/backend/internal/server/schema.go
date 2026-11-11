package server

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// bindingSchemaCustomizer 把 dto 上的 binding 标签翻译成 OpenAPI 约束。
//
// 为什么需要这一层：运行时校验器读的是 binding 标签，而文档生成器默认只认
// validate 标签（Fuego 内建的 SchemaCustomizer 就是这么写的）。
// 没有这层桥的话，接口实际要求「标题 1~128 字」，文档里却只写一个 string——
// 两份信息分开的那天起，照着文档写的前端就会收到 400。
//
// 只翻译取值约束（min/max/oneof/email）。「必填」不在这里处理：
// OpenAPI 的 required 由 json 标签的 omitempty 决定（序列化语义），
// 本项目的 dto 刻意让两者对齐，所以不需要额外搬运。
func bindingSchemaCustomizer(
	_ string, _ reflect.Type, tag reflect.StructTag, schema *openapi3.Schema,
) error {
	raw, ok := tag.Lookup("binding")
	if !ok || schema == nil {
		return nil
	}

	for _, rule := range strings.Split(raw, ",") {
		rule = strings.TrimSpace(rule)

		switch {
		case rule == "email":
			schema.Format = "email"

		case strings.HasPrefix(rule, "oneof="):
			for _, v := range strings.Fields(strings.TrimPrefix(rule, "oneof=")) {
				schema.Enum = append(schema.Enum, v)
			}

		case strings.HasPrefix(rule, "min="):
			n, err := strconv.ParseUint(strings.TrimPrefix(rule, "min="), 10, 64)
			if err != nil {
				continue
			}
			if isStringSchema(schema) {
				// 字符串的 min 是长度，数字的 min 是取值：同一个标签在两种类型上含义不同。
				schema.MinLength = n
			} else {
				schema.Min = ptrFloat(float64(n))
			}

		case strings.HasPrefix(rule, "max="):
			n, err := strconv.ParseUint(strings.TrimPrefix(rule, "max="), 10, 64)
			if err != nil {
				continue
			}
			if isStringSchema(schema) {
				schema.MaxLength = &n
			} else {
				schema.Max = ptrFloat(float64(n))
			}
		}
	}
	return nil
}

func isStringSchema(schema *openapi3.Schema) bool {
	if schema.Type == nil {
		return false
	}
	for _, t := range *schema.Type {
		if t == openapi3.TypeString {
			return true
		}
	}
	return false
}

func ptrFloat(v float64) *float64 { return &v }
